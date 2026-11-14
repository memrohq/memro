package api_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// signIn makes a dashboard user and returns their session token.
func (e *env) signIn(prefix string) (email, session string) {
	e.t.Helper()
	email = uniqueEmail(prefix)
	_, session = e.dashboardSession(email)
	return email, session
}

func inviteToken(t *testing.T, body []byte) string {
	t.Helper()
	var r struct{ Link string }
	_ = json.Unmarshal(body, &r)
	u, err := url.Parse(r.Link)
	if err != nil || u.Query().Get("token") == "" {
		t.Fatalf("no invite link in %s", body)
	}
	return u.Query().Get("token")
}

// ADR 0026: a customer's people sign in once, are invited into the
// organisation with a role, and see usage per end user but never memories.
func TestOrganisationMembersRolesAndUsage(t *testing.T) {
	e := setup(t)
	_, admin := e.signIn("staff")
	e.makePlatformAdmin(admin)

	// Staff create the organisation and invite its first owner.
	ownerEmail, owner := e.signIn("owner")
	code, body := e.do(nil, "", "POST", "/tenant", j(map[string]string{"name": "Vaultnel", "owner_email": strings.ToUpper(ownerEmail)}), bearer(admin))
	e.must(200, code, body, "create tenant with owner")
	var created struct {
		Tenant      struct{ ID string }
		OwnerInvite json.RawMessage `json:"owner_invite"`
	}
	_ = json.Unmarshal(body, &created)
	tid, ownerToken := created.Tenant.ID, inviteToken(t, created.OwnerInvite)

	_, stranger := e.signIn("stranger")
	code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": ownerToken}), bearer(stranger))
	e.must(403, code, body, "accept someone else's invite")
	code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": ownerToken}), bearer(owner))
	e.must(200, code, body, "owner accepts")
	code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": ownerToken}), bearer(owner))
	e.must(404, code, body, "invite used twice")
	code, body = e.do(nil, "", "GET", "/orgs", nil, bearer(owner))
	e.must(200, code, body, "list orgs")
	if !strings.Contains(string(body), `"role":"owner"`) || !strings.Contains(string(body), "Vaultnel") {
		t.Fatalf("orgs = %s", body)
	}

	// The owner invites an admin and a viewer.
	join := func(by, role string) (string, string) {
		email, session := e.signIn(role)
		code, body := e.do(nil, "", "POST", "/tenant/"+tid+"/invites", j(map[string]string{"email": email, "role": role}), bearer(by))
		e.must(201, code, body, "invite "+role)
		code, body = e.do(nil, "", "POST", "/invites/accept", j(map[string]string{"token": inviteToken(t, body)}), bearer(session))
		e.must(200, code, body, role+" accepts")
		return email, session
	}
	_, adm := join(owner, "admin")
	_, viewer := join(adm, "viewer")

	// What each role may do.
	code, body = e.do(nil, "", "POST", "/tenant/"+tid+"/invites", j(map[string]string{"email": uniqueEmail("x"), "role": "owner"}), bearer(adm))
	e.must(403, code, body, "admin invites an owner")
	code, body = e.do(nil, "", "POST", "/tenant/"+tid+"/invites", j(map[string]string{"email": uniqueEmail("x"), "role": "viewer"}), bearer(viewer))
	e.must(403, code, body, "viewer invites")
	code, body = e.do(nil, "", "POST", "/tenant/"+tid+"/api-keys", j(map[string]string{"name": "x"}), bearer(viewer))
	e.must(403, code, body, "viewer creates a tenant key")
	code, body = e.do(nil, "", "GET", "/tenant/"+tid+"/members", nil, bearer(stranger))
	e.must(403, code, body, "stranger lists members")
	code, body = e.do(nil, "", "GET", "/tenant/"+tid+"/members", nil, bearer(viewer))
	e.must(200, code, body, "viewer lists members")
	var members []struct {
		UserID string `json:"user_id"`
		Role   string
	}
	_ = json.Unmarshal(body, &members)
	var ownerID string
	for _, m := range members {
		if m.Role == "owner" {
			ownerID = m.UserID
		}
	}
	if len(members) != 3 || ownerID == "" {
		t.Fatalf("members = %s", body)
	}
	code, body = e.do(nil, "", "PATCH", "/tenant/"+tid+"/members/"+ownerID, j(map[string]string{"role": "viewer"}), bearer(adm))
	e.must(403, code, body, "admin demotes the owner")
	code, body = e.do(nil, "", "PATCH", "/tenant/"+tid+"/members/"+ownerID, j(map[string]string{"role": "admin"}), bearer(owner))
	e.must(409, code, body, "last owner demotes themselves")
	code, body = e.do(nil, "", "DELETE", "/tenant/"+tid+"/members/"+ownerID, nil, bearer(owner))
	e.must(409, code, body, "last owner leaves")

	// An admin issues the tenant key; two end users use it.
	code, body = e.do(nil, "", "POST", "/tenant/"+tid+"/api-keys", j(map[string]string{"name": "backend"}), bearer(adm))
	e.must(200, code, body, "admin creates a tenant key")
	var k struct{ Key string }
	_ = json.Unmarshal(body, &k)
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]string{"content": "Intro to jane at stripe went well"}), asUser(k.Key, "u_1"))
	e.must(200, code, body, "u_1 remembers")
	code, body = e.do(nil, "", "GET", "/search?mode=keyword&query=stripe", nil, asUser(k.Key, "u_1"))
	e.must(200, code, body, "u_1 searches")
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]string{"content": "Followed up with raj"}), asUser(k.Key, "u_2"))
	e.must(200, code, body, "u_2 remembers")

	// Usage per end user: for members and for the tenant's own key.
	var usage struct {
		ActiveMembers int `json:"active_members"`
		Members       []struct {
			User    string
			AgentID string `json:"agent_id"`
			Metrics map[string]int64
		}
	}
	code, body = e.do(nil, "", "GET", "/tenant/"+tid+"/usage/members", nil, bearer(viewer))
	e.must(200, code, body, "viewer reads usage")
	_ = json.Unmarshal(body, &usage)
	if usage.ActiveMembers != 2 || strings.Contains(string(body), "stripe") {
		t.Fatalf("usage = %s", body)
	}
	var u1Agent string
	for _, m := range usage.Members {
		if m.User == "u_1" {
			u1Agent = m.AgentID
			if m.Metrics["queries"] != 1 || m.Metrics["stored_tokens"] == 0 || m.Metrics["query_tokens"] == 0 {
				t.Fatalf("u_1 metrics = %v", m.Metrics)
			}
		}
	}
	code, body = e.do(nil, "", "GET", "/tenant/"+tid+"/usage/members?month="+time.Now().UTC().Format("2006-01"), nil, asUser(k.Key, "billing-job"))
	e.must(200, code, body, "tenant key reads usage")
	_, otherKey := e.tenantKey("Someone else")
	code, body = e.do(nil, "", "GET", "/tenant/"+tid+"/usage/members", nil, asUser(otherKey, "u_1"))
	e.must(403, code, body, "another tenant's key reads usage")

	// Members never read end users' memories.
	code, body = e.do(nil, "", "GET", "/memory/agent/"+u1Agent, nil, bearer(owner))
	if code == 200 && strings.Contains(string(body), "stripe") {
		t.Fatalf("an org owner read an end user's memories: %s", body)
	}

	// Anyone but the last owner may leave.
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(viewer))
	var who struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(body, &who)
	code, body = e.do(nil, "", "DELETE", "/tenant/"+tid+"/members/"+who.UserID, nil, bearer(viewer))
	e.must(204, code, body, "viewer leaves")
}
