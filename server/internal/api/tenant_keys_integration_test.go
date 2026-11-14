package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/memrohq/memro/server/internal/api"
)

// tenantKey makes a dedicated tenant and a tenant key for it, as a platform
// admin would for a new customer (ADR 0025).
func (e *env) tenantKey(name string) (tenantID, key string) {
	e.t.Helper()
	_, session := e.dashboardSession(uniqueEmail("admin"))
	e.makePlatformAdmin(session)
	code, body := e.do(nil, "", "POST", "/tenant", j(map[string]string{"name": name}), bearer(session))
	e.must(200, code, body, "create tenant")
	var t struct{ ID string }
	_ = json.Unmarshal(body, &t)
	code, body = e.do(nil, "", "POST", "/tenant/"+t.ID+"/api-keys", j(map[string]string{"name": "server"}), bearer(session))
	e.must(200, code, body, "create tenant key")
	var k struct{ Key string }
	_ = json.Unmarshal(body, &k)
	return t.ID, k.Key
}

func asUser(key, user string) map[string]string {
	h := bearer(key)
	h["X-Memro-User"] = user
	return h
}

// One customer key, many end users: each end user is created on first use and
// sees only its own memories, also across two customers using the same IDs.
func TestTenantKeyKeepsEndUsersApart(t *testing.T) {
	e := setup(t)
	tenantA, keyA := e.tenantKey("Customer A")
	_, keyB := e.tenantKey("Customer B")

	remember := func(key, user, content string) string {
		code, body := e.do(nil, "", "POST", "/memory", j(map[string]string{"content": content}), asUser(key, user))
		e.must(200, code, body, "remember as "+user)
		var m struct{ ID string }
		_ = json.Unmarshal(body, &m)
		return m.ID
	}
	sees := func(key, user, query string) string {
		code, body := e.do(nil, "", "GET", "/search?mode=keyword&query="+query, nil, asUser(key, user))
		e.must(200, code, body, "search as "+user)
		return string(body)
	}
	mem1 := remember(keyA, "u_1", "Emailed jane at stripe about the staff engineer role")
	remember(keyA, "u_2", "Emailed raj at figma, no reply yet")
	memB := remember(keyB, "u_1", "Customer B user one emailed stripe recruiting")

	if got := sees(keyA, "u_1", "stripe"); !strings.Contains(got, mem1) || strings.Contains(got, memB) {
		t.Fatalf("A/u_1 search = %s", got)
	}
	if got := sees(keyA, "u_2", "stripe"); strings.Contains(got, mem1) || strings.Contains(got, "jane") {
		t.Fatalf("A/u_2 sees another user's memory: %s", got)
	}
	if got := sees(keyB, "u_1", "stripe"); strings.Contains(got, mem1) {
		t.Fatalf("B/u_1 sees customer A's memory: %s", got)
	}

	// The same user ID maps to the same agent on every request.
	who := func(key, user string) (agent, tenant string) {
		code, body := e.do(nil, "", "GET", "/auth/whoami", nil, asUser(key, user))
		e.must(200, code, body, "whoami")
		var w struct {
			AgentID  string `json:"agent_id"`
			TenantID string `json:"tenant_id"`
		}
		_ = json.Unmarshal(body, &w)
		return w.AgentID, w.TenantID
	}
	a1, tid := who(keyA, "u_1")
	if again, _ := who(keyA, "u_1"); again != a1 || tid != tenantA {
		t.Fatalf("u_1 is agent %s then %s in tenant %s", a1, again, tid)
	}
	// End users don't share coordination claims (the tenant's other agents do).
	code, body := e.do(nil, "", "POST", "/coordination/claim", j(map[string]string{
		"resource": "candidate/jane", "intent": "emailing jane about the stripe offer"}), asUser(keyA, "u_1"))
	e.must(200, code, body, "u_1 claim")
	code, body = e.do(nil, "", "GET", "/coordination/active", nil, asUser(keyA, "u_2"))
	e.must(200, code, body, "u_2 active")
	if strings.Contains(string(body), "stripe offer") {
		t.Fatalf("u_2 sees u_1's claim: %s", body)
	}
	code, body = e.do(nil, "", "POST", "/coordination/claim", j(map[string]string{
		"resource": "candidate/jane", "intent": "x"}), asUser(keyA, "u_2"))
	e.must(200, code, body, "u_2 claim")
	if !strings.Contains(string(body), `"granted":true`) {
		t.Fatalf("u_1's claim blocks u_2: %s", body)
	}

	// A tenant key must name the end user, with a sane ID.
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(keyA))
	e.must(400, code, body, "tenant key without end user")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, asUser(keyA, "bad id/../x"))
	e.must(400, code, body, "invalid end user ID")

	// Tenant keys act only as an end user: no key or agent management.
	code, body = e.do(nil, "", "POST", "/api-keys", j(map[string]string{"name": "x"}), asUser(keyA, "u_1"))
	e.must(403, code, body, "tenant key minting keys")
	code, body = e.do(nil, "", "DELETE", "/identity/"+a1, nil, asUser(keyA, "u_1"))
	e.must(403, code, body, "tenant key deleting via /identity")
	code, body = e.do(nil, "", "POST", "/tenant/"+tenantA+"/api-keys", nil, asUser(keyA, "u_1"))
	e.must(403, code, body, "tenant key minting tenant keys")

	// Deleting an end user removes its memories; the others keep theirs.
	code, body = e.do(nil, "", "DELETE", "/end-user", nil, asUser(keyA, "u_1"))
	e.must(204, code, body, "delete end user")
	code, body = e.do(nil, "", "DELETE", "/end-user", nil, asUser(keyA, "u_1"))
	e.must(404, code, body, "delete an end user twice")
	var n int
	_ = e.st.Pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM agent_identities WHERE external_id = 'u_1' AND tenant_id = $1`, tenantA).Scan(&n)
	if n != 0 {
		t.Fatalf("deleting an unknown end user created it")
	}
	if got := sees(keyA, "u_1", "stripe"); strings.Contains(got, mem1) {
		t.Fatalf("deleted user's memory still found: %s", got)
	}
	if got := sees(keyA, "u_2", "figma"); !strings.Contains(got, "Emailed raj") {
		t.Fatalf("other user lost memories: %s", got)
	}
	if got := sees(keyB, "u_1", "stripe"); !strings.Contains(got, memB) {
		t.Fatalf("other customer's u_1 was deleted: %s", got)
	}

	// Ordinary API keys cannot use /end-user.
	_, session := e.dashboardSession(uniqueEmail("plain"))
	code, body = e.do(nil, "", "DELETE", "/end-user", nil, bearer(e.createKey(session)))
	e.must(http.StatusForbidden, code, body, "plain key deleting end user")
}

func TestTenantKeysAreForPlatformAdminsOnly(t *testing.T) {
	e := setup(t)
	tenantID, key := e.tenantKey("Customer C")
	_, session := e.dashboardSession(uniqueEmail("member"))

	code, body := e.do(nil, "", "POST", "/tenant/"+tenantID+"/api-keys", nil, bearer(session))
	e.must(403, code, body, "non-admin creating a tenant key")

	_, admin := e.dashboardSession(uniqueEmail("admin2"))
	e.makePlatformAdmin(admin)
	code, body = e.do(nil, "", "POST", "/tenant/00000000-0000-0000-0000-000000000000/api-keys", nil, bearer(admin))
	e.must(400, code, body, "tenant key for the default tenant")

	code, body = e.do(nil, "", "GET", "/tenant/"+tenantID+"/api-keys", nil, bearer(admin))
	e.must(200, code, body, "list tenant keys")
	var keys []struct{ ID string }
	_ = json.Unmarshal(body, &keys)
	if len(keys) != 1 {
		t.Fatalf("keys = %s", body)
	}
	code, body = e.do(nil, "", "DELETE", "/tenant/"+tenantID+"/api-keys/"+keys[0].ID, nil, bearer(admin))
	e.must(204, code, body, "revoke tenant key")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, asUser(key, "u_1"))
	e.must(401, code, body, "revoked tenant key")
}

// A customer's backend sends all its users' requests from one address: valid
// tenant-key requests don't use up that address's limit, failed ones do.
func TestTenantKeyRequestsSkipTheAddressLimit(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.RateLimitMax = 5 })
	_, key := e.tenantKey("Customer D") // 3 of the address's 5 requests
	for i := range 10 {
		code, body := e.do(nil, "", "GET", "/auth/whoami", nil, asUser(key, fmt.Sprintf("u_%d", i)))
		e.must(200, code, body, "valid tenant-key request")
	}
	var codes []int
	for range 3 {
		code, _ := e.do(nil, "", "GET", "/auth/whoami", nil, asUser("memro_sk_wrong", "u_1"))
		codes = append(codes, code)
	}
	if fmt.Sprint(codes) != "[401 401 429]" {
		t.Fatalf("failed tenant-key requests got %v, want [401 401 429]", codes)
	}
}
