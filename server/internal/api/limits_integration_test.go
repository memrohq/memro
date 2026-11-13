package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/memrohq/memro/server/internal/api"
)

// raw sends a request and returns the status and the Memro-Limit headers.
func (e *env) raw(method, path, body string, hdr map[string]string) (code int, limit, warning string) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	for h, v := range hdr {
		req.Header.Set(h, v)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Memro-Limit"), resp.Header.Get("Memro-Limit-Warning")
}

// memory16 is a memory of 16 bytes: 4 stored tokens.
func memory16(i int) string { return fmt.Sprintf(`{"content":"memory no %06d"}`, i) }

// ADR 0028: an organisation's plan replaces the default per-agent cap for its
// end users, and only staff can change it.
func TestTenantLimitsReplaceDefaultCap(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.MemorySoftCap = 2 })
	staffEmail, staff := e.signIn("staff")
	e.makePlatformAdmin(staff)
	ownerEmail, owner := e.signIn("owner")
	code, body := e.do(nil, "", "POST", "/tenant", j(map[string]string{"name": "Limits Co", "owner_email": ownerEmail}), bearer(staff))
	e.must(200, code, body, "create tenant")
	var created struct {
		Tenant      struct{ ID string }
		OwnerInvite json.RawMessage `json:"owner_invite"`
	}
	_ = json.Unmarshal(body, &created)
	base := "/tenant/" + created.Tenant.ID
	code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": inviteToken(t, created.OwnerInvite)}), bearer(owner))
	e.must(200, code, body, "owner accepts")
	code, body = e.do(nil, "", "POST", base+"/api-keys", j(map[string]string{"name": "server"}), bearer(staff))
	e.must(200, code, body, "tenant key")
	var k struct{ Key string }
	_ = json.Unmarshal(body, &k)
	n := 0
	write := func(user string) (int, string) {
		n++
		c, l, _ := e.raw("POST", "/memory", memory16(n), asUser(k.Key, user))
		return c, l
	}
	limits := func(sess, method string, v any) (int, []byte) {
		var b []byte
		if v != nil {
			b = j(v)
		}
		return e.do(nil, "", method, base+"/limits", b, bearer(sess))
	}

	// Without a plan the server's default cap (2 memories per agent) applies.
	c1, _ := write("u1")
	c2, _ := write("u1")
	c3, _ := write("u1")
	if c1 != 200 || c2 != 200 || c3 != 403 {
		t.Fatalf("default cap: %d %d %d", c1, c2, c3)
	}

	// Members read the plan but can't change it; staff can, even as members.
	code, body = limits(owner, "GET", nil)
	e.must(200, code, body, "owner reads limits")
	if !strings.Contains(string(body), `"limits":null`) {
		t.Fatalf("limits = %s", body)
	}
	code, body = limits(owner, "PUT", map[string]any{"plan": "enterprise"})
	e.must(403, code, body, "owner sets limits")
	code, body = e.do(nil, "", "POST", base+"/invites", j(map[string]string{"email": staffEmail, "role": "owner"}), bearer(owner))
	e.must(201, code, body, "invite staff as owner")
	code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": inviteToken(t, body)}), bearer(staff))
	e.must(200, code, body, "staff joins as owner")

	// A per-user token limit replaces the memory count cap.
	code, body = limits(staff, "PUT", map[string]any{"plan": "custom", "max_stored_tokens_per_user": 10})
	e.must(200, code, body, "custom limit")
	for i, want := range []int{200, 200, 403} {
		if c, why := write("u9"); c != want || (want == 403 && why != "stored_tokens_per_user") {
			t.Fatalf("u9 write %d: %d %q", i, c, why)
		}
	}

	// Bad input is refused.
	for _, bad := range []map[string]any{{"plan": "gold"}, {"plan": "custom", "max_stored_tokens_total": 0}, {"plan": "trial", "trial_ends_at": "tomorrow"}} {
		code, body = limits(staff, "PUT", bad)
		e.must(400, code, body, "bad limits")
	}

	// Enterprise has no limits; /usage reports no memory cap.
	code, body = limits(staff, "PUT", map[string]any{"plan": "enterprise"})
	e.must(200, code, body, "enterprise")
	if c, _ := write("u9"); c != 200 {
		t.Fatalf("enterprise write: %d", c)
	}
	code, body = e.do(nil, "", "GET", "/usage", nil, asUser(k.Key, "u9"))
	e.must(200, code, body, "usage")
	if !strings.Contains(string(body), `"memory_limit":0`) {
		t.Fatalf("usage = %s", body)
	}

	// Resetting returns the organisation to the default cap.
	code, body = limits(staff, "DELETE", nil)
	e.must(204, code, body, "reset")
	if c, _ := write("u1"); c != 403 {
		t.Fatalf("u1 over the default cap after reset: %d", c)
	}
}

// A trial is the public Free tier shared by all of an organisation's users:
// stored tokens, queries, 3 seats, no images; one per company; read-only
// after it ends, then deleted 60 days later after warnings (ADR 0028).
func TestTrialPlan(t *testing.T) {
	var deps *api.Deps
	e := setup(t, func(d *api.Deps) { deps = d })
	jobs := api.NewServer(*deps) // runs the hourly jobs against the same database
	ctx := context.Background()
	_, staff := e.signIn("staff")
	e.makePlatformAdmin(staff)
	company := fmt.Sprintf("trial%d.test", time.Now().UnixNano())
	at := func(p string) string { return fmt.Sprintf("%s-%d@%s", p, time.Now().UnixNano(), company) }
	_, owner := e.dashboardSession(at("owner"))
	type created struct {
		TenantID     string `json:"tenant_id"`
		TrialStarted bool   `json:"trial_started"`
	}
	newOrg := func(session, name string) (created, string) {
		code, body := e.do(nil, "", "POST", "/orgs", j(map[string]string{"name": name}), bearer(session))
		e.must(201, code, body, "create org")
		var c created
		_ = json.Unmarshal(body, &c)
		code, body = e.do(nil, "", "POST", "/tenant/"+c.TenantID+"/api-keys", j(map[string]string{"name": "server"}), bearer(session))
		e.must(200, code, body, "trial org gets a key")
		var k struct{ Key string }
		_ = json.Unmarshal(body, &k)
		return c, k.Key
	}

	org, key := newOrg(owner, "Trial Co")
	if !org.TrialStarted {
		t.Fatal("first org got no trial")
	}
	base := "/tenant/" + org.TenantID
	put := func(v map[string]any) {
		t.Helper()
		code, body := e.do(nil, "", "PUT", base+"/limits", j(v), bearer(staff))
		e.must(200, code, body, "set limits")
	}

	// A second org from the same company gets no second trial.
	_, colleague := e.dashboardSession(at("colleague"))
	again, key2 := newOrg(colleague, "Trial Co again")
	if again.TrialStarted {
		t.Fatalf("second trial for %s", company)
	}
	if c, why, _ := e.raw("POST", "/memory", memory16(0), asUser(key2, "u1")); c != 403 || why != "trial_ended" {
		t.Fatalf("write without a trial: %d %q", c, why)
	}

	// Users share the stored-token pool; warnings start at 80%.
	put(map[string]any{"plan": "trial", "max_stored_tokens_total": 10})
	if c, _, warn := e.raw("POST", "/memory", memory16(1), asUser(key, "u1")); c != 200 || warn != "" {
		t.Fatalf("first write: %d %q", c, warn)
	}
	if c, _, warn := e.raw("POST", "/memory", memory16(2), asUser(key, "u2")); c != 200 || warn != "stored_tokens_total=80%" {
		t.Fatalf("second write: %d %q", c, warn)
	}
	if c, why, _ := e.raw("POST", "/memory", memory16(3), asUser(key, "u3")); c != 403 || why != "stored_tokens_total" {
		t.Fatalf("pool full: %d %q", c, why)
	}

	// Queries are shared per month too.
	put(map[string]any{"plan": "trial", "max_queries_monthly": 2})
	for i, want := range []int{200, 200, 403} {
		if c, why, _ := e.raw("GET", "/search?mode=keyword&query=memory", "", asUser(key, fmt.Sprintf("u%d", i+1))); c != want || (want == 403 && why != "queries_monthly") {
			t.Fatalf("search %d: %d %q", i, c, why)
		}
	}

	// No image analysis, and 3 seats counting pending invitations.
	if c, why, _ := e.raw("POST", "/upload/image", "", asUser(key, "u1")); c != 403 || why != "images" {
		t.Fatalf("image on trial: %d %q", c, why)
	}
	for i, want := range []int{201, 201, 403} {
		code, body := e.do(nil, "", "POST", base+"/invites", j(map[string]string{"email": at(fmt.Sprintf("seat%d", i)), "role": "viewer"}), bearer(owner))
		e.must(want, code, body, fmt.Sprintf("invite %d", i))
	}

	// The hourly snapshot records what each user holds, for billing.
	if err := e.st.SnapshotStoredTokens(ctx); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(nil, "", "GET", base+"/usage/members", nil, bearer(owner))
	e.must(200, code, body, "member usage")
	if !strings.Contains(string(body), `"stored_tokens_held":4`) {
		t.Fatalf("member usage = %s", body)
	}

	// After the trial: read-only, warned, then deleted 60 days on.
	ended := func(daysAgo int) {
		put(map[string]any{"plan": "trial", "max_queries_monthly": nil, "trial_ends_at": time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour).UTC().Format(time.RFC3339)})
	}
	notice := func() string {
		code, body := e.do(nil, "", "GET", base+"/limits", nil, bearer(owner))
		e.must(200, code, body, "limits")
		var r struct {
			Limits struct {
				TrialNotice string `json:"trial_notice"`
			}
		}
		_ = json.Unmarshal(body, &r)
		return r.Limits.TrialNotice
	}
	ended(1)
	if c, why, _ := e.raw("POST", "/memory", memory16(4), asUser(key, "u1")); c != 403 || why != "trial_ended" {
		t.Fatalf("after trial: %d %q", c, why)
	}
	if c, _, _ := e.raw("GET", "/search?mode=keyword&query=memory", "", asUser(key, "u1")); c != 200 {
		t.Fatalf("read after trial: %d", c)
	}
	api.AdvanceTrials(jobs, ctx)
	if n := notice(); n != "ended" {
		t.Fatalf("notice after the end = %q", n)
	}
	ended(50) // deletion in 10 days: only the 14-day warning is sent
	api.AdvanceTrials(jobs, ctx)
	if n := notice(); n != "d14" {
		t.Fatalf("notice 10 days before deletion = %q", n)
	}
	ended(61)
	api.AdvanceTrials(jobs, ctx)
	if n := notice(); n != "deleted" {
		t.Fatalf("notice after deletion = %q", n)
	}

	// The admin portal shows the emptied organisation, still on the trial.
	code, body = e.do(nil, "", "GET", "/admin/tenants", nil, bearer(owner))
	e.must(403, code, body, "owner opens admin portal")
	code, body = e.do(nil, "", "GET", "/admin/tenants", nil, bearer(staff))
	e.must(200, code, body, "admin tenants")
	var tenants []struct {
		ID           string
		StoredTokens int64 `json:"stored_tokens"`
		EndUsers     int   `json:"end_users"`
		Limits       *struct{ Plan string }
	}
	_ = json.Unmarshal(body, &tenants)
	found := false
	for _, tn := range tenants {
		if tn.ID == org.TenantID {
			found = tn.StoredTokens == 0 && tn.EndUsers == 0 && tn.Limits != nil && tn.Limits.Plan == "trial"
		}
	}
	if !found {
		t.Fatalf("admin tenants after deletion: %s", body)
	}
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(staff))
	e.must(200, code, body, "staff whoami")
	if !strings.Contains(string(body), `"platform_admin":true`) {
		t.Fatalf("staff whoami = %s", body)
	}
}
