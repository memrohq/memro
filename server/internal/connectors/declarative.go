package connectors

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/connectors/dce"
	"github.com/memrohq/memro/server/internal/domain"
)

// declarativeProvider wraps a DCE manifest into Memro's native provider interface.
type declarativeProvider struct {
	m        *Manager
	manifest *dce.Manifest
}

func newDeclarativeProvider(m *Manager, manifest *dce.Manifest) *declarativeProvider {
	return &declarativeProvider{
		m:        m,
		manifest: manifest,
	}
}

func (p *declarativeProvider) enabled() bool {
	return p.m.sealer != nil
}

func (p *declarativeProvider) info() ProviderInfo {
	name := p.manifest.Metadata.Name
	return ProviderInfo{
		Name:             name,
		Enabled:          p.enabled(),
		SourcesManagedBy: "token",
		Kind:             "token",
		DisplayName:      dce.FormattedDisplayName(name),
		Category:         dce.CategoryFor(name),
		Description:      p.manifest.Metadata.Description,
		SecretRef:        p.manifest.Spec.Auth.SecretRef,
	}
}

func (p *declarativeProvider) authorizeURL(state string) string {
	return ""
}

func (p *declarativeProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	token := strings.TrimSpace(q.Get("token"))
	if token == "" {
		return nil, &UserError{"API token cannot be empty."}
	}

	return &authorized{
		accountID:   p.manifest.Metadata.Name,
		accountName: p.manifest.Metadata.Name,
		userID:      "token_user",
		userName:    p.manifest.Metadata.Name + " Token",
		creds: map[string]any{
			"token": token,
		},
	}, nil
}

type connectorItemSink struct {
	ctx      context.Context
	c        *conn
	m        *Manager
	manifest *dce.Manifest
}

func (s *connectorItemSink) IngestItem(ctx context.Context, norm *dce.NormalizedItem) error {
	header := fmt.Sprintf("[%s · %s]", strings.ToUpper(s.manifest.Metadata.Name), norm.Title)
	if norm.Title == "" {
		header = fmt.Sprintf("[%s · %s]", strings.ToUpper(s.manifest.Metadata.Name), norm.ID)
	}

	body := strings.TrimSpace(norm.Content)
	if body == "" && norm.Raw != nil {
		var b strings.Builder
		for k, v := range norm.Raw {
			if v != nil && k != "id" && k != "gid" {
				b.WriteString(fmt.Sprintf("- **%s**: %v\n", k, v))
			}
		}
		body = strings.TrimSpace(b.String())
	}
	if norm.Title != "" && !strings.Contains(body, norm.Title) {
		body = norm.Title + "\n\n" + body
	}
	if body == "" {
		body = norm.ID
	}

	evDate := norm.EventDate
	if evDate.IsZero() {
		evDate = time.Now().UTC()
	}

	it := item{
		Key:       fmt.Sprintf("%s:%s", s.manifest.Metadata.Name, norm.ID),
		SourceID:  s.manifest.Metadata.Name,
		Header:    header,
		Body:      body,
		Type:      domain.Episodic,
		EventDate: &evDate,
		Meta:      norm.Raw,
	}

	return s.m.storeItem(ctx, s.c, it)
}

func (p *declarativeProvider) handle(ctx context.Context, c *conn, j *job) error {
	token := c.str("token")
	if token == "" {
		return &revokedError{"API token missing or revoked."}
	}

	secretRef := p.manifest.Spec.Auth.SecretRef
	if secretRef == "" {
		secretRef = "API_KEY"
	}

	secrets := dce.MapSecretResolver{
		secretRef: token,
	}

	runtime := dce.NewRuntime(p.m.http, secrets)
	sink := &connectorItemSink{
		ctx:      ctx,
		c:        c,
		m:        p.m,
		manifest: p.manifest,
	}

	cursor := ""
	_ = p.m.deps.Pool.QueryRow(ctx, `SELECT cursor FROM connector_sources WHERE connection_id = $1 AND external_id = $2`,
		c.ID, p.manifest.Metadata.Name).Scan(&cursor)

	res, err := runtime.Execute(ctx, p.manifest, sink, cursor)
	if err != nil {
		return err
	}

	if res.NewCheckpoint != "" {
		_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET cursor = $3, last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
			c.ID, p.manifest.Metadata.Name, res.NewCheckpoint)
	} else {
		_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_sources SET last_synced_at = NOW() WHERE connection_id = $1 AND external_id = $2`,
			c.ID, p.manifest.Metadata.Name)
	}
	_, _ = p.m.deps.Pool.Exec(ctx, `UPDATE connector_connections SET last_synced_at = NOW() WHERE id = $1`, c.ID)

	return nil
}

func (p *declarativeProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
	return nil
}

func (p *declarativeProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	return nil
}

func (p *declarativeProvider) manageURL(c *Connection) string {
	return ""
}
