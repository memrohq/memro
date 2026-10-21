package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/connectors/dce"
	"github.com/memrohq/memro/server/internal/domain"
)

// Catalog connectors (ADR 0014): services described by a manifest in
// dce/catalog/, connected by pasting an API token instead of an OAuth
// sign-in. They share the job queue, ingestion (chunk diffs, tombstones),
// sealed credentials and quotas with the native connectors.

// catalogPages is how many pages one sync job fetches before handing the
// rest of the pass to another job, so one large source cannot hold a worker.
const catalogPages = 5

type catalogProvider struct {
	m   *Manager
	man *dce.Manifest
	rt  *dce.Runtime
}

func (p *catalogProvider) enabled() bool { return p.m.sealer != nil }

func (p *catalogProvider) info() ProviderInfo {
	md := p.man.Metadata
	fields := make([]FieldInfo, 0, len(p.man.Spec.Fields))
	for _, f := range p.man.Spec.Fields {
		fields = append(fields, FieldInfo{Name: f.Name, Label: f.Label, Help: f.Help, Placeholder: f.Placeholder,
			Secret: f.Secret, Optional: f.Optional})
	}
	return ProviderInfo{Name: md.Name, Enabled: p.enabled(), SourcesManagedBy: "memro", Kind: "token",
		DisplayName: md.DisplayName, Category: md.Category, Description: md.Description, DocsURL: md.DocsURL, Fields: fields}
}

func (p *catalogProvider) authorizeURL(string) string { return "" }

func (p *catalogProvider) complete(context.Context, url.Values) (*authorized, error) {
	return nil, &UserError{p.man.Metadata.DisplayName + " connects with an API token, not a sign-in link."}
}

func (p *catalogProvider) manageURL(*Connection) string { return p.man.Metadata.DocsURL }

// revoke: the user revokes the token in the service's settings; Memro drops
// its sealed copy when the connection is deleted.
func (p *catalogProvider) revoke(context.Context, *conn, bool) error { return nil }

func (p *catalogProvider) call(c *conn) dce.Call {
	cfg := map[string]string{}
	for k, v := range c.creds {
		if s, ok := v.(string); ok {
			cfg[k] = s
		}
	}
	return dce.Call{Manifest: p.man, Config: cfg}
}

func (p *catalogProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.sync(ctx, c, j.Key)
	}
	return &permanentError{"unknown " + p.man.Metadata.Name + " job " + j.Kind}
}

func (p *catalogProvider) refreshSources(ctx context.Context, c *conn) error {
	list, err := p.rt.ListSources(ctx, p.call(c))
	if err != nil {
		return p.jobError(err)
	}
	srcs := make([]Source, 0, len(list))
	for _, s := range list {
		srcs = append(srcs, Source{ID: s.ID, Name: s.Name})
	}
	// Without declared sources there is one, "all": enabled straight away.
	// With them the user picks in the dashboard, like Slack channels.
	all := p.man.Spec.Sources == nil
	if err := p.m.replaceSources(ctx, c.ID, srcs, all); err != nil {
		return err
	}
	if all {
		return p.m.enqueue(ctx, nil, c.ID, jobSync, "all", nil, 0)
	}
	return nil
}

func (p *catalogProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
	existing, err := p.m.sources(ctx, c.ID)
	if err != nil {
		return err
	}
	byID := map[string]Source{}
	for _, s := range existing {
		byID[s.ID] = s
	}
	for id, on := range enabled {
		s, ok := byID[id]
		if !ok {
			return &UserError{"Unknown source " + id + ". Refresh the list."}
		}
		if err := p.m.setSourceEnabled(ctx, c.ID, id, on); err != nil {
			return err
		}
		if on && !s.Enabled {
			if err := p.m.enqueue(ctx, nil, c.ID, jobSync, id, nil, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// syncState is a source's position, kept in connector_sources.cursor:
// the page cursor of the pass in progress, the checkpoint of the last
// finished pass, and the highest checkpoint seen so far in this pass. The
// checkpoint only advances when a pass finishes, so an interrupted pass never
// skips items.
type syncState struct {
	Op         int    `json:"op,omitempty"`
	Cursor     string `json:"c,omitempty"`
	Checkpoint string `json:"k,omitempty"`
	High       string `json:"h,omitempty"`
}

func (p *catalogProvider) sync(ctx context.Context, c *conn, sourceID string) error {
	src, err := p.m.source(ctx, c.ID, sourceID)
	if err != nil {
		return err
	}
	if src == nil || !src.Enabled {
		return nil // turned off since the job was queued
	}
	var st syncState
	if src.cursor != "" {
		_ = json.Unmarshal([]byte(src.cursor), &st)
	}
	call := p.call(c)
	call.SourceID, call.Source = src.ID, src.Name
	ops := p.man.Spec.Operations
	if st.Op >= len(ops) {
		st.Op = 0
	}
	for page := 0; page < catalogPages; page++ {
		call.Cursor, call.Checkpoint = st.Cursor, st.Checkpoint
		res, err := p.rt.Page(ctx, call, ops[st.Op])
		if err != nil {
			return p.jobError(err)
		}
		// A first sync imports only the backfill window (CONNECTOR_BACKFILL_DAYS);
		// a later pass of a newest-first source stops at the last checkpoint,
		// less the manifest's overlap (late writes, items that change later).
		cutoff := time.Time{}
		if st.Checkpoint == "" {
			cutoff = p.m.now().AddDate(0, 0, -p.m.cfg.BackfillDays)
		} else if p.man.Spec.Sync.NewestFirst {
			if t := dce.ParseDate(st.Checkpoint); t.Year() > 2000 {
				cutoff = t.Add(-p.man.Spec.Sync.OverlapDuration())
			}
		}
		older := 0
		for _, it := range res.Items {
			if !cutoff.IsZero() && !it.EventDate.IsZero() && it.EventDate.Before(cutoff) {
				older++
				continue
			}
			if err := p.m.storeItem(ctx, c, p.item(src, it)); err != nil {
				return err
			}
		}
		st.High = laterOf(st.High, res.MaxCheckpoint)
		st.Cursor = res.NextCursor
		if p.man.Spec.Sync.NewestFirst && len(res.Items) > 0 && older == len(res.Items) {
			st.Cursor = "" // newest first and this page is all older: the window is done
		}
		if st.Cursor != "" {
			continue
		}
		// This operation's pass is done; move to the next operation, and
		// after the last one, commit the checkpoint.
		if st.Op+1 < len(ops) {
			st.Op++
			continue
		}
		st = syncState{Checkpoint: laterOf(st.Checkpoint, st.High)}
		return p.save(ctx, c, src.ID, st, true)
	}
	if err := p.save(ctx, c, src.ID, st, false); err != nil {
		return err
	}
	return p.m.enqueue(ctx, nil, c.ID, jobSync, src.ID, nil, 0)
}

func (p *catalogProvider) save(ctx context.Context, c *conn, sourceID string, st syncState, finished bool) error {
	b, _ := json.Marshal(st)
	return p.m.saveCursor(ctx, c.ID, sourceID, string(b), finished)
}

func laterOf(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, tb := dce.ParseDate(a), dce.ParseDate(b)
	if !ta.IsZero() && !tb.IsZero() {
		if tb.After(ta) {
			return b
		}
		return a
	}
	if b > a {
		return b
	}
	return a
}

// item renders a catalog item for ingestion. The header names the service,
// the source and the date, so each memory says where it came from.
func (p *catalogProvider) item(src *Source, it *dce.NormalizedItem) item {
	header := "[" + p.man.Metadata.DisplayName
	if src.ID != "all" && src.Name != "" {
		header += " · " + src.Name
	}
	var when *time.Time
	if !it.EventDate.IsZero() {
		t := it.EventDate
		when = &t
		header += " · " + t.Format("2006-01-02")
	}
	header += "]"
	body := it.Content
	if it.URL != "" {
		body += "\n" + it.URL
	}
	typ := domain.Semantic
	if it.MemoryType == "episodic" {
		typ = domain.Episodic
	}
	return item{
		Key: src.ID + "/" + it.ID, SourceID: src.ID, Header: header, Body: body, Type: typ, EventDate: when,
		Meta: map[string]any{"source": p.man.Metadata.Name, "external_id": it.ID, "url": it.URL, "title": it.Title},
	}
}

// jobError classifies a failure during a sync job: a refused token revokes
// the connection (the user reconnects with a new one), a rate limit waits,
// anything else retries with backoff until the connection shows an error.
func (p *catalogProvider) jobError(err error) error {
	var he *dce.HTTPError
	if errors.As(err, &he) {
		switch he.Status {
		case 401:
			return &revokedError{p.man.Metadata.DisplayName + " no longer accepts the token. Reconnect with a new one."}
		case 429:
			return &rateLimitedError{after: time.Minute}
		}
	}
	return p.wrap(err)
}

// wrap turns API failures the user can fix into messages for the dashboard.
func (p *catalogProvider) wrap(err error) error {
	var he *dce.HTTPError
	if errors.As(err, &he) {
		switch he.Status {
		case 401:
			return &UserError{p.man.Metadata.DisplayName + " refused the token (401). Check it, or create a new one."}
		case 403:
			return &UserError{p.man.Metadata.DisplayName + " says the token cannot read this (403). Check its scopes."}
		case 404:
			return &UserError{p.man.Metadata.DisplayName + " could not find that (404). Check the account details."}
		}
	}
	if errors.Is(err, dce.ErrSSRFBlocked) {
		return &permanentError{err.Error()}
	}
	return err
}

// ConnectToken connects a catalog service with the fields the user entered
// (token and account details): it checks them, asks the service who the
// token belongs to, seals them and starts the first sync.
// legacyToken is a bare token from an older dashboard; it fills the
// manifest's token field.
func (m *Manager) ConnectToken(ctx context.Context, providerName, agentID string, tenantID uuid.UUID, fields map[string]string, legacyToken string) (*Connection, error) {
	pr, err := m.provider(providerName)
	if err != nil {
		return nil, err
	}
	p, ok := pr.(*catalogProvider)
	if !ok {
		return nil, &UserError{"This connector uses a sign-in link, not a token."}
	}
	if legacyToken != "" && p.man.Spec.Auth.SecretRef != "" {
		if fields == nil {
			fields = map[string]string{}
		}
		if fields[p.man.Spec.Auth.SecretRef] == "" {
			fields[p.man.Spec.Auth.SecretRef] = legacyToken
		}
	}
	cfg := map[string]string{}
	for _, f := range p.man.Spec.Fields {
		if v := strings.TrimSpace(fields[f.Name]); v != "" {
			cfg[f.Name] = v
		}
	}
	if err := dce.CheckConfig(p.man, cfg); err != nil {
		return nil, &UserError{err.Error()}
	}
	accountID, accountName, err := p.rt.Verify(ctx, dce.Call{Manifest: p.man, Config: cfg})
	if err != nil {
		if w := p.wrap(err); w != err {
			return nil, w
		}
		return nil, &UserError{"Could not reach " + p.man.Metadata.DisplayName + " with these details."}
	}
	if accountID == "" {
		// Name the account by its non-secret details, never by the token.
		h := sha256.New()
		for _, f := range p.man.Spec.Fields {
			if !f.Secret {
				fmt.Fprintf(h, "%s=%s;", f.Name, cfg[f.Name])
			}
		}
		accountID = hex.EncodeToString(h.Sum(nil))[:16]
	}
	if accountName == "" {
		accountName = p.man.Metadata.DisplayName
	}
	creds := map[string]any{}
	for k, v := range cfg {
		creds[k] = v
	}
	id, err := m.saveConnection(ctx, agentID, tenantID, providerName, &authorized{accountID: accountID, accountName: accountName, creds: creds})
	if err != nil {
		return nil, err
	}
	if err := m.enqueue(ctx, nil, id, jobSources, jobSources, nil, 0); err != nil {
		return nil, err
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	c.ManageURL = p.manageURL(c)
	return c, nil
}
