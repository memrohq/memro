package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/memrohq/memro/server/internal/metrics"
)

// Bot scopes: read channel lists and history, names of people, and join
// public channels the user picks. Nothing is ever posted.
var slackScopes = []string{
	"channels:history", "channels:read", "channels:join",
	"groups:history", "groups:read",
	"users:read", "team:read",
}

const (
	slackMaxSkew     = 5 * time.Minute
	slackRenderDelay = 15 * time.Second // coalesces bursts of messages in one channel-day
	slackPageSize    = 200
)

type slackProvider struct {
	m         *Manager
	refreshMu sync.Mutex
}

func (p *slackProvider) enabled() bool {
	return p.m.sealer != nil && p.m.cfg.SlackClientID != "" && p.m.cfg.SlackClientSecret != "" && p.m.cfg.SlackSigningSecret != ""
}

func (p *slackProvider) info() ProviderInfo {
	return ProviderInfo{Name: ProviderSlack, Enabled: p.enabled(), SourcesManagedBy: "memro"}
}

func (p *slackProvider) manageURL(*Connection) string { return "" }

func (p *slackProvider) authorizeURL(state string) string {
	q := url.Values{
		"client_id":    {p.m.cfg.SlackClientID},
		"scope":        {strings.Join(slackScopes, ",")},
		"redirect_uri": {p.m.callbackURL(ProviderSlack)},
		"state":        {state},
	}
	return p.m.cfg.SlackOAuthURL + "?" + q.Encode()
}

// --- API client -----------------------------------------------------------------

type slackError struct{ code string }

func (e *slackError) Error() string { return "slack: " + e.code }

// call posts a Web API method and decodes the response into out.
func (p *slackProvider) call(ctx context.Context, token, method string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.m.cfg.SlackAPIURL+"/"+method, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return &rateLimitedError{after: retryAfter(resp.Header, time.Minute)}
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("slack %s: HTTP %d", method, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return err
	}
	var base struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &base); err != nil {
		return fmt.Errorf("slack %s: invalid response (HTTP %d)", method, resp.StatusCode)
	}
	if !base.OK {
		switch base.Error {
		case "ratelimited":
			return &rateLimitedError{after: time.Minute}
		case "invalid_auth", "not_authed", "account_inactive", "token_revoked", "token_expired",
			"team_access_not_granted", "org_login_required", "ekm_access_denied":
			return &revokedError{"Slack no longer accepts this connection (" + base.Error + "). Reconnect it."}
		case "missing_scope":
			return &revokedError{"The Slack app is missing a permission. Reconnect to grant it."}
		}
		return &slackError{base.Error}
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

func retryAfter(h http.Header, def time.Duration) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && s >= 0 {
		return max(time.Duration(s)*time.Second, time.Second)
	}
	return def
}

// token returns the connection's bot token, refreshing it first when Slack
// token rotation is on and it is about to expire.
func (p *slackProvider) token(ctx context.Context, c *conn) (string, error) {
	exp, _ := c.creds["expires_at"].(float64)
	if exp == 0 || time.Unix(int64(exp), 0).After(p.m.now().Add(5*time.Minute)) {
		return c.str("bot_token"), nil
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	// Another job may have refreshed it while we waited.
	fresh, err := p.m.connection(ctx, c.ID)
	if err != nil {
		return "", err
	}
	if fc, err := p.m.open(ctx, fresh); err == nil {
		c.creds = fc.creds
		if e, _ := c.creds["expires_at"].(float64); time.Unix(int64(e), 0).After(p.m.now().Add(5 * time.Minute)) {
			return c.str("bot_token"), nil
		}
	}
	var r slackOAuthResponse
	err = p.call(ctx, "", "oauth.v2.access", url.Values{
		"client_id": {p.m.cfg.SlackClientID}, "client_secret": {p.m.cfg.SlackClientSecret},
		"grant_type": {"refresh_token"}, "refresh_token": {c.str("refresh_token")},
	}, &r)
	var se *slackError
	if errors.As(err, &se) && (se.code == "invalid_refresh_token" || se.code == "invalid_grant") {
		return "", &revokedError{"Slack authorization expired. Reconnect Slack."}
	}
	if err != nil {
		return "", err
	}
	oldRefresh := c.str("refresh_token")
	c.creds["bot_token"] = r.AccessToken
	c.creds["refresh_token"] = r.RefreshToken
	c.creds["expires_at"] = float64(p.m.now().Unix() + r.ExpiresIn)
	if err := c.saveCreds(ctx); err != nil {
		return "", err
	}
	// Every connection to this workspace holds the same bot token.
	others, _ := p.m.connectionsForAccount(ctx, ProviderSlack, c.AccountID, false)
	for _, o := range others {
		if o.ID == c.ID {
			continue
		}
		if oc, err := p.m.open(ctx, o); err == nil && oc.str("refresh_token") == oldRefresh {
			oc.creds["bot_token"], oc.creds["refresh_token"], oc.creds["expires_at"] = c.creds["bot_token"], c.creds["refresh_token"], c.creds["expires_at"]
			_ = oc.saveCreds(ctx)
		}
	}
	return r.AccessToken, nil
}

// --- OAuth ------------------------------------------------------------------------

type slackOAuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	BotUserID    string `json:"bot_user_id"`
	AppID        string `json:"app_id"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Team         *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	IsEnterpriseInstall bool `json:"is_enterprise_install"`
	AuthedUser          struct {
		ID string `json:"id"`
	} `json:"authed_user"`
}

func (p *slackProvider) complete(ctx context.Context, q url.Values) (*authorized, error) {
	code := q.Get("code")
	if code == "" {
		return nil, &UserError{"Slack did not return an authorization code."}
	}
	var r slackOAuthResponse
	if err := p.call(ctx, "", "oauth.v2.access", url.Values{
		"client_id": {p.m.cfg.SlackClientID}, "client_secret": {p.m.cfg.SlackClientSecret},
		"code": {code}, "redirect_uri": {p.m.callbackURL(ProviderSlack)},
	}, &r); err != nil {
		slog.Warn("slack oauth exchange", "err", err)
		return nil, &UserError{"Slack sign-in failed. Please try again."}
	}
	if r.IsEnterpriseInstall || r.Team == nil || r.Team.ID == "" {
		return nil, &UserError{"Install Memro to a single Slack workspace; organization-wide installs aren't supported."}
	}
	if r.TokenType != "bot" || r.AccessToken == "" {
		return nil, &UserError{"Slack did not return a bot token."}
	}
	creds := map[string]any{
		"bot_token": r.AccessToken, "bot_user_id": r.BotUserID, "authed_user_id": r.AuthedUser.ID, "app_id": r.AppID,
	}
	if r.RefreshToken != "" {
		creds["refresh_token"] = r.RefreshToken
		creds["expires_at"] = float64(p.m.now().Unix() + r.ExpiresIn)
	}
	name := r.AuthedUser.ID
	var u slackUserInfo
	if err := p.call(ctx, r.AccessToken, "users.info", url.Values{"user": {r.AuthedUser.ID}}, &u); err == nil {
		name = u.displayName()
	}
	return &authorized{accountID: r.Team.ID, accountName: r.Team.Name, userID: r.AuthedUser.ID, userName: name, creds: creds}, nil
}

func (p *slackProvider) revoke(ctx context.Context, c *conn, lastForAccount bool) error {
	// The bot token is shared by everyone who connected this workspace, so it
	// is only revoked when the last of them disconnects.
	if !lastForAccount {
		return nil
	}
	return p.call(ctx, c.str("bot_token"), "auth.revoke", url.Values{}, nil)
}

// --- People -----------------------------------------------------------------------

type slackUserInfo struct {
	User struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Deleted bool   `json:"deleted"`
		IsBot   bool   `json:"is_bot"`
		Profile struct {
			DisplayName string `json:"display_name"`
			RealName    string `json:"real_name"`
		} `json:"profile"`
	} `json:"user"`
}

func (u slackUserInfo) displayName() string {
	for _, n := range []string{u.User.Profile.DisplayName, u.User.Profile.RealName, u.User.Name} {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return u.User.ID
}

type cachedName struct {
	name    string
	expires time.Time
}

// userName resolves a Slack user ID to a display name, cached for an hour.
func (p *slackProvider) userName(ctx context.Context, c *conn, userID string) string {
	if userID == "" {
		return "unknown"
	}
	key := c.AccountID + "/" + userID
	m := p.m
	m.namesMu.Lock()
	if n, ok := m.names[key]; ok && n.expires.After(m.now()) {
		m.namesMu.Unlock()
		return n.name
	}
	m.namesMu.Unlock()
	name := userID
	token, err := p.token(ctx, c)
	if err == nil {
		var u slackUserInfo
		if err := p.call(ctx, token, "users.info", url.Values{"user": {userID}}, &u); err == nil {
			name = u.displayName()
		}
	}
	m.namesMu.Lock()
	if len(m.names) > 50_000 {
		clear(m.names)
	}
	m.names[key] = cachedName{name, m.now().Add(time.Hour)}
	m.namesMu.Unlock()
	return name
}

// --- Channels ---------------------------------------------------------------------

type slackChannel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived"`
	IsMember   bool   `json:"is_member"`
}

// isMember reports whether the Slack user who connected is in the channel.
// Private channels are only offered and synced for their members, so one
// person cannot read a private channel through someone else's install.
func (p *slackProvider) isMember(ctx context.Context, c *conn, token, channel string) (bool, error) {
	user := c.str("authed_user_id")
	cursor := ""
	for range 50 {
		var r struct {
			Members  []string `json:"members"`
			Metadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := p.call(ctx, token, "conversations.members", url.Values{"channel": {channel}, "limit": {"1000"}, "cursor": {cursor}}, &r); err != nil {
			return false, err
		}
		for _, mbr := range r.Members {
			if mbr == user {
				return true, nil
			}
		}
		if cursor = r.Metadata.NextCursor; cursor == "" {
			return false, nil
		}
	}
	return false, nil
}

func (p *slackProvider) refreshSources(ctx context.Context, c *conn) error {
	token, err := p.token(ctx, c)
	if err != nil {
		return err
	}
	var list []Source
	cursor := ""
	for range 50 {
		var r struct {
			Channels []slackChannel `json:"channels"`
			Metadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := p.call(ctx, token, "conversations.list", url.Values{
			"types": {"public_channel,private_channel"}, "exclude_archived": {"true"}, "limit": {"1000"}, "cursor": {cursor},
		}, &r); err != nil {
			return err
		}
		for _, ch := range r.Channels {
			if ch.IsPrivate {
				ok, err := p.isMember(ctx, c, token, ch.ID)
				if err != nil || !ok {
					if err != nil {
						slog.Info("slack private channel skipped", "connection", c.ID, "err", err)
					}
					continue
				}
			}
			list = append(list, Source{ID: ch.ID, Name: ch.Name, Private: ch.IsPrivate})
		}
		if cursor = r.Metadata.NextCursor; cursor == "" {
			break
		}
	}
	return p.m.replaceSources(ctx, c.ID, list, false)
}

func (p *slackProvider) setSources(ctx context.Context, c *conn, enabled map[string]bool) error {
	if len(enabled) == 0 {
		return nil
	}
	srcs, err := p.m.sources(ctx, c.ID)
	if err != nil {
		return err
	}
	byID := map[string]Source{}
	for _, s := range srcs {
		byID[s.ID] = s
	}
	token, err := p.token(ctx, c)
	if err != nil {
		return userFacing(err)
	}
	for id, on := range enabled {
		s, ok := byID[id]
		if !ok {
			return &UserError{"Unknown channel " + truncate(id, 40) + ". Refresh the channel list and try again."}
		}
		if on && !s.Enabled {
			if s.Private {
				member, err := p.isMember(ctx, c, token, id)
				if err != nil {
					return userFacing(err)
				}
				if !member {
					return &UserError{"You're not a member of #" + s.Name + "."}
				}
			} else if err := p.call(ctx, token, "conversations.join", url.Values{"channel": {id}}, nil); err != nil {
				var se *slackError
				if errors.As(err, &se) && se.code == "is_archived" {
					return &UserError{"#" + s.Name + " is archived."}
				}
				return userFacing(err)
			}
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

// userFacing turns a provider failure during a dashboard action into a
// message the user can act on.
func userFacing(err error) error {
	var rv *revokedError
	var rl *rateLimitedError
	switch {
	case errors.As(err, &rv):
		return &UserError{rv.msg}
	case errors.As(err, &rl):
		return &UserError{"Slack is rate limiting requests. Try again in a minute."}
	}
	slog.Warn("connector action", "err", err)
	return &UserError{"Slack request failed. Please try again."}
}

// --- Jobs ---------------------------------------------------------------------------

func (p *slackProvider) handle(ctx context.Context, c *conn, j *job) error {
	switch j.Kind {
	case jobSources:
		return p.refreshSources(ctx, c)
	case jobSync:
		return p.syncChannel(ctx, c, j.Key)
	case jobThread:
		return p.syncThread(ctx, c, j.Payload)
	case jobEvent:
		return p.applyEvent(ctx, c, j.Payload)
	case jobRender:
		return p.render(ctx, c, j.Key)
	}
	return &permanentError{"unknown slack job " + j.Kind}
}

// slackMessage is a message as returned by the Web API and Events API.
type slackMessage struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	TS         string `json:"ts"`
	ThreadTS   string `json:"thread_ts"`
	User       string `json:"user"`
	BotID      string `json:"bot_id"`
	Text       string `json:"text"`
	ReplyCount int    `json:"reply_count"`
	LatestRepl string `json:"latest_reply"`
	Edited     *struct {
		TS string `json:"ts"`
	} `json:"edited"`
	Files []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"files"`
}

// keep reports whether a message is conversation (not joins, bots, etc.).
func (msg *slackMessage) keep() bool {
	switch msg.Subtype {
	case "", "thread_broadcast", "file_share", "me_message":
	default:
		return false
	}
	return msg.BotID == "" && msg.TS != "" && (strings.TrimSpace(msg.Text) != "" || len(msg.Files) > 0)
}

type slackCursor struct {
	HW     string `json:"hw,omitempty"`     // newest message ts fully synced
	RunHW  string `json:"run_hw,omitempty"` // newest ts seen in the run in progress
	Oldest string `json:"oldest,omitempty"` // lower bound of the run in progress
	Page   string `json:"page,omitempty"`   // next page of the run in progress
}

// syncChannel processes one page of channel history and queues the next.
func (p *slackProvider) syncChannel(ctx context.Context, c *conn, channel string) error {
	src, err := p.m.source(ctx, c.ID, channel)
	if err != nil || src == nil || !src.Enabled {
		return err
	}
	token, err := p.token(ctx, c)
	if err != nil {
		return err
	}
	if src.Private {
		member, err := p.isMember(ctx, c, token, channel)
		if err != nil {
			return err
		}
		if !member {
			slog.Info("slack: user left private channel; syncing stopped", "connection", c.ID, "channel", channel)
			return p.m.setSourceEnabled(ctx, c.ID, channel, false)
		}
	}
	var cur slackCursor
	_ = json.Unmarshal([]byte(src.cursor), &cur)
	if cur.Page == "" {
		cur.Oldest = cur.HW
		if cur.Oldest == "" {
			start := p.m.now().AddDate(0, 0, -p.m.cfg.BackfillDays)
			cur.Oldest = strconv.FormatInt(start.Unix(), 10) + ".000000"
		}
		cur.RunHW = cur.HW
	}
	var r struct {
		Messages []slackMessage `json:"messages"`
		HasMore  bool           `json:"has_more"`
		Metadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	err = p.call(ctx, token, "conversations.history", url.Values{
		"channel": {channel}, "oldest": {cur.Oldest}, "limit": {strconv.Itoa(slackPageSize)}, "cursor": {cur.Page},
	}, &r)
	var se *slackError
	if errors.As(err, &se) && (se.code == "channel_not_found" || se.code == "not_in_channel" || se.code == "is_archived") {
		slog.Info("slack channel no longer readable; syncing stopped", "connection", c.ID, "channel", channel, "reason", se.code)
		return p.m.setSourceEnabled(ctx, c.ID, channel, false)
	}
	if err != nil {
		return err
	}
	var keep []slackMessage
	for _, msg := range r.Messages {
		if tsLess(cur.RunHW, msg.TS) {
			cur.RunHW = msg.TS
		}
		if msg.ReplyCount > 0 && tsLess(cur.Oldest, msg.LatestRepl) {
			if err := p.m.enqueue(ctx, nil, c.ID, jobThread, channel+"/"+msg.TS,
				map[string]string{"channel": channel, "ts": msg.TS}, 0); err != nil {
				return err
			}
		}
		if msg.keep() {
			keep = append(keep, msg)
		}
	}
	if err := p.merge(ctx, c, channel, src.Name, keep, false); err != nil {
		return err
	}
	if r.HasMore && r.Metadata.NextCursor != "" {
		cur.Page = r.Metadata.NextCursor
		if err := p.m.saveCursor(ctx, c.ID, channel, mustJSON(cur), false); err != nil {
			return err
		}
		return p.m.enqueue(ctx, nil, c.ID, jobSync, channel, nil, 0)
	}
	return p.m.saveCursor(ctx, c.ID, channel, mustJSON(slackCursor{HW: cur.RunHW}), true)
}

func (p *slackProvider) syncThread(ctx context.Context, c *conn, payload json.RawMessage) error {
	var t struct{ Channel, TS string }
	if err := json.Unmarshal(payload, &t); err != nil {
		return &permanentError{"bad thread job"}
	}
	src, err := p.m.source(ctx, c.ID, t.Channel)
	if err != nil || src == nil || !src.Enabled {
		return err
	}
	token, err := p.token(ctx, c)
	if err != nil {
		return err
	}
	var all []slackMessage
	cursor := ""
	for range 20 {
		var r struct {
			Messages []slackMessage `json:"messages"`
			Metadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		err := p.call(ctx, token, "conversations.replies", url.Values{
			"channel": {t.Channel}, "ts": {t.TS}, "limit": {strconv.Itoa(slackPageSize)}, "cursor": {cursor},
		}, &r)
		var se *slackError
		if errors.As(err, &se) && se.code == "thread_not_found" {
			return nil
		}
		if err != nil {
			return err
		}
		for _, msg := range r.Messages {
			if msg.keep() {
				all = append(all, msg)
			}
		}
		if cursor = r.Metadata.NextCursor; cursor == "" {
			break
		}
	}
	return p.merge(ctx, c, t.Channel, src.Name, all, false)
}

// --- Documents ----------------------------------------------------------------------

// slackDoc is one channel-day: every message whose thread started that day
// (UTC), so a thread stays together in one item.
type slackDoc struct {
	Channel     string                `json:"channel"`
	ChannelName string                `json:"channel_name"`
	Day         string                `json:"day"`
	Messages    map[string]slackEntry `json:"messages"`
}

type slackEntry struct {
	User    string `json:"user"`
	Name    string `json:"name"`
	Text    string `json:"text"`
	Thread  string `json:"thread,omitempty"`
	V       string `json:"v"` // version: edit or delete time; newer wins
	Deleted bool   `json:"deleted,omitempty"`
}

func slackItemKey(channel, rootTS string) (string, string) {
	day := tsTime(rootTS).UTC().Format("2006-01-02")
	return channel + "/" + day, day
}

// merge applies messages (new, edited or deleted) to their channel-day
// documents and queues those days to be re-rendered.
func (p *slackProvider) merge(ctx context.Context, c *conn, channel, channelName string, msgs []slackMessage, deleted bool) error {
	byItem := map[string][]slackMessage{}
	for _, msg := range msgs {
		root := msg.ThreadTS
		if root == "" {
			root = msg.TS
		}
		key, _ := slackItemKey(channel, root)
		byItem[key] = append(byItem[key], msg)
	}
	for key, group := range byItem {
		entries := make(map[string]slackEntry, len(group))
		for _, msg := range group {
			v := msg.TS
			if msg.Edited != nil && tsLess(v, msg.Edited.TS) {
				v = msg.Edited.TS
			}
			e := slackEntry{User: msg.User, Thread: msg.ThreadTS, V: v, Deleted: deleted}
			if !deleted {
				e.Name = p.userName(ctx, c, msg.User)
				e.Text = p.formatText(ctx, c, msg.Text)
				for _, f := range msg.Files {
					name := f.Title
					if name == "" {
						name = f.Name
					}
					e.Text = strings.TrimSpace(e.Text + " [file: " + name + "]")
				}
			}
			entries[msg.TS] = e
		}
		if err := p.updateDoc(ctx, c, key, channel, channelName, entries); err != nil {
			return err
		}
	}
	return nil
}

func (p *slackProvider) updateDoc(ctx context.Context, c *conn, key, channel, channelName string, entries map[string]slackEntry) error {
	tx, err := p.m.deps.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, _, err := lockItem(ctx, tx, c.ID, key, channel); err != nil {
		return err
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM connector_items WHERE connection_id = $1 AND item_key = $2`, c.ID, key).Scan(&raw); err != nil {
		return err
	}
	doc := slackDoc{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	doc.Channel, doc.ChannelName, doc.Day = channel, channelName, strings.TrimPrefix(key, channel+"/")
	if doc.Messages == nil {
		doc.Messages = map[string]slackEntry{}
	}
	changed := false
	for ts, e := range entries {
		old, ok := doc.Messages[ts]
		if ok && !tsLess(old.V, e.V) && !(old.V == e.V && e.Deleted && !old.Deleted) {
			continue // what we have is as new or newer
		}
		if e.Deleted {
			e.User, e.Name, e.Text, e.Thread = old.User, "", "", old.Thread
		}
		doc.Messages[ts] = e
		changed = true
	}
	if !changed {
		return tx.Commit(ctx)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE connector_items SET document = $3, updated_at = NOW()
		WHERE connection_id = $1 AND item_key = $2`, c.ID, key, b); err != nil {
		return err
	}
	if err := p.m.enqueue(ctx, tx, c.ID, jobRender, key, nil, slackRenderDelay); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// render rebuilds a channel-day's memories from its document.
func (p *slackProvider) render(ctx context.Context, c *conn, key string) error {
	var raw []byte
	var sourceID string
	err := p.m.deps.Pool.QueryRow(ctx, `SELECT document, source_id FROM connector_items WHERE connection_id = $1 AND item_key = $2`,
		c.ID, key).Scan(&raw, &sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc slackDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return &permanentError{"corrupt slack document " + key}
	}
	body, first := renderSlackDoc(&doc)
	if body == "" {
		return p.m.removeItem(ctx, c, key)
	}
	header := fmt.Sprintf("[Slack #%s · %s · %s]", doc.ChannelName, doc.Day, c.AccountName)
	return p.m.storeItem(ctx, c, item{
		Key: key, SourceID: sourceID, Header: header, Body: body, Type: "episodic", EventDate: &first,
		Meta: map[string]any{"team": c.AccountID, "channel": doc.Channel, "channel_name": doc.ChannelName, "day": doc.Day,
			"url": "https://app.slack.com/client/" + c.AccountID + "/" + doc.Channel},
	})
}

// --- Events API ---------------------------------------------------------------------

// VerifySlackSignature checks Slack's request signature (v0) and timestamp.
func VerifySlackSignature(secret string, h http.Header, body []byte, now time.Time) bool {
	ts := h.Get("X-Slack-Request-Timestamp")
	sig := h.Get("X-Slack-Signature")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return false
	}
	if d := now.Sub(time.Unix(sec, 0)); d > slackMaxSkew || d < -slackMaxSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	return hmac.Equal([]byte("v0="+hex.EncodeToString(mac.Sum(nil))), []byte(sig))
}

// SlackEvents handles a request from the Slack Events API and returns the
// status and JSON body to reply with. Work is queued, never done inline,
// so Slack gets its answer within its 3-second limit.
func (m *Manager) SlackEvents(ctx context.Context, h http.Header, body []byte) (int, any) {
	if !m.slack.enabled() {
		return http.StatusNotFound, map[string]string{"error": "slack connector not configured"}
	}
	if !VerifySlackSignature(m.cfg.SlackSigningSecret, h, body, m.now()) {
		metrics.ConnectorWebhooks.WithLabelValues(ProviderSlack, "bad_signature").Inc()
		return http.StatusUnauthorized, map[string]string{"error": "invalid signature"}
	}
	var env struct {
		Type      string          `json:"type"`
		Challenge string          `json:"challenge"`
		TeamID    string          `json:"team_id"`
		Event     json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid JSON"}
	}
	if env.Type == "url_verification" {
		return http.StatusOK, map[string]string{"challenge": env.Challenge}
	}
	if env.Type != "event_callback" || env.TeamID == "" {
		return http.StatusOK, map[string]bool{"ok": true}
	}
	var ev struct {
		Type    string `json:"type"`
		Channel any    `json:"channel"`
		Subtype string `json:"subtype"`
	}
	_ = json.Unmarshal(env.Event, &ev)
	result := "ignored"
	var err error
	switch ev.Type {
	case "app_uninstalled":
		result = "uninstalled"
		err = m.revokeAccount(ctx, ProviderSlack, env.TeamID, "Memro was removed from this Slack workspace.")
	case "tokens_revoked":
		var t struct {
			Tokens struct {
				Bot []string `json:"bot"`
			} `json:"tokens"`
		}
		_ = json.Unmarshal(env.Event, &t)
		if len(t.Tokens.Bot) > 0 {
			result = "revoked"
			err = m.revokeAccount(ctx, ProviderSlack, env.TeamID, "Slack revoked Memro's access to this workspace.")
		}
	case "channel_rename", "group_rename":
		var r struct {
			Channel struct{ ID, Name string } `json:"channel"`
		}
		_ = json.Unmarshal(env.Event, &r)
		_, err = m.deps.Pool.Exec(ctx, `UPDATE connector_sources s SET name = $3 FROM connector_connections c
			WHERE s.connection_id = c.id AND c.provider = 'slack' AND c.account_id = $1 AND s.external_id = $2`,
			env.TeamID, r.Channel.ID, r.Channel.Name)
		result = "renamed"
	case "message":
		channel, _ := ev.Channel.(string)
		if channel == "" {
			break
		}
		result = "queued"
		err = m.queueSlackEvent(ctx, env.TeamID, channel, env.Event)
	}
	if err != nil {
		slog.Error("slack event", "type", ev.Type, "err", err)
		metrics.ConnectorWebhooks.WithLabelValues(ProviderSlack, "error").Inc()
		return http.StatusInternalServerError, map[string]string{"error": "temporary failure"} // Slack retries
	}
	metrics.ConnectorWebhooks.WithLabelValues(ProviderSlack, result).Inc()
	return http.StatusOK, map[string]bool{"ok": true}
}

// queueSlackEvent queues a message event for every connection that syncs
// the channel.
func (m *Manager) queueSlackEvent(ctx context.Context, team, channel string, event json.RawMessage) error {
	rows, err := m.deps.Pool.Query(ctx, `SELECT c.id FROM connector_connections c
		JOIN connector_sources s ON s.connection_id = c.id AND s.external_id = $2 AND s.enabled
		WHERE c.provider = 'slack' AND c.account_id = $1 AND c.status = 'active'`, team, channel)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.enqueue(ctx, nil, id, jobEvent, "", event, 0); err != nil {
			return err
		}
	}
	return nil
}

func (p *slackProvider) applyEvent(ctx context.Context, c *conn, payload json.RawMessage) error {
	var ev struct {
		slackMessage
		Channel    string        `json:"channel"`
		EventTS    string        `json:"event_ts"`
		Message    *slackMessage `json:"message"`
		DeletedTS  string        `json:"deleted_ts"`
		PrevMessag *slackMessage `json:"previous_message"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return &permanentError{"bad slack event"}
	}
	src, err := p.m.source(ctx, c.ID, ev.Channel)
	if err != nil || src == nil || !src.Enabled {
		return err
	}
	switch ev.Subtype {
	case "message_changed":
		if ev.Message == nil {
			return nil
		}
		msg := *ev.Message
		if msg.Edited == nil {
			// Not a text edit (e.g. a reply count changed); nothing to update.
			return nil
		}
		if !msg.keep() {
			return nil
		}
		return p.merge(ctx, c, ev.Channel, src.Name, []slackMessage{msg}, false)
	case "message_deleted":
		msg := slackMessage{TS: ev.DeletedTS, Edited: &struct {
			TS string `json:"ts"`
		}{TS: ev.EventTS}}
		if ev.PrevMessag != nil {
			msg.ThreadTS = ev.PrevMessag.ThreadTS
		}
		if msg.TS == "" {
			return nil
		}
		return p.merge(ctx, c, ev.Channel, src.Name, []slackMessage{msg}, true)
	}
	msg := ev.slackMessage
	if !msg.keep() {
		return nil
	}
	return p.merge(ctx, c, ev.Channel, src.Name, []slackMessage{msg}, false)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
