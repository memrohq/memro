package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Regression tests for the 2026-10-20 authorization review (ADR 0012).

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func (e *env) createKey(session string) string {
	e.t.Helper()
	code, body := e.do(nil, "", "POST", "/api-keys", j(map[string]string{"name": "test"}), bearer(session))
	e.must(200, code, body, "create api key")
	var k struct{ Key string }
	_ = json.Unmarshal(body, &k)
	return k.Key
}

func (e *env) makePlatformAdmin(session string) {
	e.t.Helper()
	code, body := e.do(nil, "", "GET", "/auth/whoami", nil, bearer(session))
	e.must(200, code, body, "whoami")
	var who struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(body, &who)
	if _, err := e.st.Pool.Exec(context.Background(), `INSERT INTO platform_admins (user_id) VALUES ($1)`, who.UserID); err != nil {
		e.t.Fatal(err)
	}
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@example.com", prefix, time.Now().UnixNano())
}

// A key leaked from a tool's config must not destroy the agent.
func TestAPIKeysCannotDeleteTheirAgent(t *testing.T) {
	e := setup(t)
	a, session := e.dashboardSession(uniqueEmail("del"))
	key := e.createKey(session)

	code, body := e.do(nil, "", "DELETE", "/identity/"+a.id, nil, bearer(key))
	e.must(403, code, body, "delete agent with api key")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(key))
	e.must(200, code, body, "agent still exists after refused delete")

	// The agent's own signature still may (SDK self-owned agents).
	code, body = e.do(&a, "v2", "DELETE", "/identity/"+a.id, nil, nil)
	e.must(204, code, body, "delete agent with its signature")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(key))
	e.must(401, code, body, "key of a deleted agent")
}

func TestDashboardSessionCanDeleteItsAgent(t *testing.T) {
	e := setup(t)
	a, session := e.dashboardSession(uniqueEmail("del-session"))
	code, body := e.do(nil, "", "DELETE", "/identity/"+a.id, nil, bearer(session))
	e.must(204, code, body, "delete agent with session")
}

func TestTenantEndpointsRequirePlatformAdmin(t *testing.T) {
	e := setup(t)
	defaultTenant := uuid.Nil.String()

	user, userSession := e.dashboardSession(uniqueEmail("user"))
	userKey := e.createKey(userSession)
	sdk := newAgent(t)
	e.register(sdk)
	e.remember(sdk, "usage that should only be visible to staff", "")

	callers := map[string]func(method, path string, body []byte) (int, []byte){
		"session": func(m, p string, b []byte) (int, []byte) { return e.do(nil, "", m, p, b, bearer(userSession)) },
		"api key": func(m, p string, b []byte) (int, []byte) { return e.do(nil, "", m, p, b, bearer(userKey)) },
		"signed":  func(m, p string, b []byte) (int, []byte) { return e.do(&sdk, "v2", m, p, b, nil) },
		"signed as dashboard agent": func(m, p string, b []byte) (int, []byte) {
			return e.do(&user, "v2", m, p, b, nil)
		},
	}
	for name, call := range callers {
		code, body := call("POST", "/tenant", j(map[string]string{"name": "Acme"}))
		e.must(403, code, body, name+": create tenant")
		code, body = call("GET", "/tenant/"+defaultTenant, nil)
		e.must(403, code, body, name+": read default tenant")
		code, body = call("GET", "/tenant/"+defaultTenant+"/usage?hours=1", nil)
		e.must(403, code, body, name+": read default tenant usage")
	}

	// Platform admins act through a session only.
	_, adminSession := e.dashboardSession(uniqueEmail("admin"))
	e.makePlatformAdmin(adminSession)
	adminKey := e.createKey(adminSession)
	code, body := e.do(nil, "", "POST", "/tenant", j(map[string]string{"name": "Acme"}), bearer(adminKey))
	e.must(403, code, body, "admin's api key creates tenant")

	code, body = e.do(nil, "", "POST", "/tenant", j(map[string]string{"name": "Acme"}), bearer(adminSession))
	e.must(200, code, body, "admin creates tenant")
	var tenant struct{ ID string }
	_ = json.Unmarshal(body, &tenant)
	code, body = e.do(nil, "", "GET", "/tenant/"+tenant.ID, nil, bearer(adminSession))
	e.must(200, code, body, "admin reads tenant")
	code, body = e.do(nil, "", "GET", "/tenant/"+defaultTenant+"/usage?hours=1000000", nil, bearer(adminSession))
	e.must(200, code, body, "admin reads default tenant usage")
	var usage []struct {
		Metric      string
		Value       int64
		PeriodHours int `json:"period_hours"`
	}
	_ = json.Unmarshal(body, &usage)
	for _, u := range usage {
		if u.PeriodHours != 90*24 {
			t.Fatalf("period_hours = %d, want it capped at 90 days", u.PeriodHours)
		}
		if u.Metric == "memories_stored" && u.Value == 0 {
			t.Fatalf("usage not recorded: %s", body)
		}
	}

	// A member of a dedicated tenant reads its own tenant, and no other.
	tid := uuid.MustParse(tenant.ID)
	member := newAgent(t)
	if err := e.st.CreateIdentity(context.Background(), member.id, nil, tid); err != nil {
		t.Fatal(err)
	}
	code, body = e.do(&member, "v2", "GET", "/tenant/"+tenant.ID, nil, nil)
	e.must(200, code, body, "member reads own tenant")
	code, body = e.do(&member, "v2", "GET", "/tenant/"+tenant.ID+"/usage", nil, nil)
	e.must(200, code, body, "member reads own tenant usage")
	code, body = e.do(&member, "v2", "GET", "/tenant/"+defaultTenant, nil, nil)
	e.must(403, code, body, "member reads default tenant")
	code, body = e.do(&member, "v2", "POST", "/tenant", j(map[string]string{"name": "Other"}), nil)
	e.must(403, code, body, "member creates tenant")
}

// The public identity lookup says an agent exists, not who owns it.
func TestPublicIdentityLookupHidesTheOwner(t *testing.T) {
	e := setup(t)
	a, _ := e.dashboardSession(uniqueEmail("lookup"))
	code, body := e.do(nil, "", "GET", "/identity/"+a.id, nil, nil)
	e.must(200, code, body, "public identity lookup")
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["id"] != a.id || len(got) != 2 || got["created_at"] == nil {
		t.Fatalf("public identity = %s, want only id and created_at", body)
	}
}

func TestAgentRegistrationIsRateLimitedPerAddress(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		e.register(newAgent(t))
	}
	code, body := e.do(nil, "", "POST", "/identity", j(map[string]string{"public_key": newAgent(t).id}), nil)
	e.must(429, code, body, "21st registration in an hour")
}

// A webhook added with an API key goes when the key is revoked, so a leaked
// key cannot leave a hook behind that keeps receiving new memories.
func TestRevokingAKeyRemovesItsWebhooks(t *testing.T) {
	e := setup(t)
	_, session := e.dashboardSession(uniqueEmail("hooks"))
	leaked, kept := e.createKey(session), e.createKey(session)
	hook := func(tok, url string) {
		t.Helper()
		code, body := e.do(nil, "", "POST", "/hooks", j(map[string]string{"event_type": "memory_created", "url": url}), bearer(tok))
		e.must(201, code, body, "register hook "+url)
	}
	hook(leaked, "https://attacker.example.com/a")
	hook(leaked, "https://hooks.example.com/both") // later re-registered by the owner
	hook(kept, "https://hooks.example.com/kept-key")
	hook(session, "https://hooks.example.com/session")
	hook(session, "https://hooks.example.com/both")

	code, body := e.do(nil, "", "GET", "/api-keys", nil, bearer(session))
	e.must(200, code, body, "list keys")
	var keys []struct{ ID, Name, Prefix string }
	_ = json.Unmarshal(body, &keys)
	var leakedID string
	for _, k := range keys {
		if len(k.Prefix) > 0 && len(leaked) >= len(k.Prefix) && leaked[:len(k.Prefix)] == k.Prefix {
			leakedID = k.ID
		}
	}
	if leakedID == "" {
		t.Fatalf("leaked key not found in %s", body)
	}
	code, body = e.do(nil, "", "DELETE", "/api-keys/"+leakedID, nil, bearer(session))
	e.must(204, code, body, "revoke leaked key")

	code, body = e.do(nil, "", "GET", "/hooks/memory_created", nil, bearer(session))
	e.must(200, code, body, "list hooks")
	var urls []string
	_ = json.Unmarshal(body, &urls)
	want := map[string]bool{"https://hooks.example.com/both": true, "https://hooks.example.com/kept-key": true, "https://hooks.example.com/session": true}
	if len(urls) != len(want) {
		t.Fatalf("hooks after revoking the key = %v, want %v", urls, want)
	}
	for _, u := range urls {
		if !want[u] {
			t.Fatalf("hooks after revoking the key = %v, want %v", urls, want)
		}
	}
}
