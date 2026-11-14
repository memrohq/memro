package api_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// ADR 0029: the staff console sees all of Memro, for staff only, and staff
// manage who else is staff.
func TestStaffConsole(t *testing.T) {
	e := setup(t)
	_, staff := e.signIn("staff")
	e.makePlatformAdmin(staff)
	userEmail, user := e.signIn("customer")

	for _, path := range []string{"/admin/overview", "/admin/users", "/admin/tenants"} {
		code, body := e.do(nil, "", "GET", path, nil, bearer(user))
		e.must(403, code, body, "customer opens "+path)
	}
	code, body := e.do(nil, "", "GET", "/admin/overview", nil, bearer(staff))
	e.must(200, code, body, "overview")
	var o struct{ Users, Staff int }
	_ = json.Unmarshal(body, &o)
	if o.Users < 2 || o.Staff < 1 {
		t.Fatalf("overview = %s", body)
	}

	code, body = e.do(nil, "", "GET", "/admin/users?q="+url.QueryEscape(userEmail), nil, bearer(staff))
	e.must(200, code, body, "find user")
	var users []struct {
		ID, Email string
		Staff     bool
	}
	_ = json.Unmarshal(body, &users)
	if len(users) != 1 || users[0].Email != userEmail || users[0].Staff {
		t.Fatalf("users = %s", body)
	}
	id := users[0].ID

	// Staff make someone staff, who can then open the console; nobody
	// removes their own rights.
	code, body = e.do(nil, "", "PUT", "/admin/staff/"+id, nil, bearer(user))
	e.must(403, code, body, "customer makes themselves staff")
	code, body = e.do(nil, "", "PUT", "/admin/staff/"+id, nil, bearer(staff))
	e.must(204, code, body, "grant staff")
	code, body = e.do(nil, "", "GET", "/admin/overview", nil, bearer(user))
	e.must(200, code, body, "new staff opens the console")
	code, body = e.do(nil, "", "DELETE", "/admin/staff/"+id, nil, bearer(user))
	e.must(409, code, body, "revoke own rights")
	code, body = e.do(nil, "", "DELETE", "/admin/staff/"+id, nil, bearer(staff))
	e.must(204, code, body, "revoke staff")
	code, body = e.do(nil, "", "GET", "/admin/users", nil, bearer(user))
	e.must(403, code, body, "former staff")
}

// Staff see everything about a person or organisation, and can suspend, sign
// out, revoke keys and delete; suspension stops every kind of request.
func TestStaffAccountActions(t *testing.T) {
	e := setup(t)
	_, staff := e.signIn("staff")
	e.makePlatformAdmin(staff)
	email, user := e.signIn("customer")
	code, body := e.do(nil, "", "GET", "/auth/whoami", nil, bearer(user))
	e.must(200, code, body, "whoami")
	var who struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(body, &who)
	base := "/admin/users/" + who.UserID

	// The detail page.
	code, body = e.do(nil, "", "GET", base, nil, bearer(staff))
	e.must(200, code, body, "user detail")
	var d struct {
		Email     string
		AgentList []struct{ ID string } `json:"agent_list"`
		SignIns   []string              `json:"sign_ins"`
	}
	_ = json.Unmarshal(body, &d)
	if d.Email != email || len(d.AgentList) != 1 || len(d.SignIns) == 0 {
		t.Fatalf("user detail = %s", body)
	}
	code, body = e.do(nil, "", "GET", base, nil, bearer(user))
	e.must(403, code, body, "customer opens a detail page")

	// Suspension refuses their requests, and lifting it restores them.
	code, body = e.do(nil, "", "POST", base+"/suspend", j(map[string]string{"reason": "unpaid invoice"}), bearer(staff))
	e.must(200, code, body, "suspend")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(user))
	e.must(403, code, body, "suspended request")
	if !strings.Contains(string(body), "unpaid invoice") {
		t.Fatalf("suspended message = %s", body)
	}
	code, body = e.do(nil, "", "POST", base+"/unsuspend", nil, bearer(staff))
	e.must(200, code, body, "unsuspend")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(user))
	e.must(200, code, body, "after unsuspend")

	// Signing out everywhere ends their session.
	code, body = e.do(nil, "", "POST", base+"/sign-out", nil, bearer(staff))
	e.must(200, code, body, "sign out")
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(user))
	e.must(401, code, body, "signed-out session")

	// Staff can't act on themselves; deleting needs the email typed.
	code, body = e.do(nil, "", "GET", "/auth/whoami", nil, bearer(staff))
	e.must(200, code, body, "staff whoami")
	var me struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(body, &me)
	code, body = e.do(nil, "", "POST", "/admin/users/"+me.UserID+"/suspend", nil, bearer(staff))
	e.must(409, code, body, "suspend yourself")
	code, body = e.do(nil, "", "DELETE", base, j(map[string]string{"confirm": "someone@else.com"}), bearer(staff))
	e.must(400, code, body, "delete without confirming")
	code, body = e.do(nil, "", "DELETE", base, j(map[string]string{"confirm": email}), bearer(staff))
	e.must(200, code, body, "delete")
	code, body = e.do(nil, "", "GET", base, nil, bearer(staff))
	e.must(404, code, body, "deleted person")

	// An organisation: its detail page, a suspension that stops its server
	// key, revoking keys, and deletion.
	tid, key := e.tenantKey("Suspended Co")
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]string{"content": "hello"}), asUser(key, "u1"))
	e.must(200, code, body, "remember")
	tbase := "/admin/tenants/" + tid
	code, body = e.do(nil, "", "GET", tbase, nil, bearer(staff))
	e.must(200, code, body, "org detail")
	if !strings.Contains(string(body), `"end_users":1`) {
		t.Fatalf("org detail = %s", body)
	}
	code, body = e.do(nil, "", "POST", tbase+"/suspend", nil, bearer(staff))
	e.must(200, code, body, "suspend org")
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]string{"content": "hello again"}), asUser(key, "u1"))
	e.must(403, code, body, "suspended org's key")
	code, body = e.do(nil, "", "POST", tbase+"/unsuspend", nil, bearer(staff))
	e.must(200, code, body, "unsuspend org")
	code, body = e.do(nil, "", "POST", tbase+"/revoke-keys", nil, bearer(staff))
	e.must(200, code, body, "revoke org keys")
	code, body = e.do(nil, "", "POST", "/memory", j(map[string]string{"content": "after revoke"}), asUser(key, "u1"))
	e.must(401, code, body, "revoked key")
	code, body = e.do(nil, "", "DELETE", tbase, j(map[string]string{"confirm": "Suspended Co"}), bearer(staff))
	e.must(200, code, body, "delete org")
	code, body = e.do(nil, "", "GET", tbase, nil, bearer(staff))
	e.must(404, code, body, "deleted org")

	// The activity feed shows what staff did.
	code, body = e.do(nil, "", "GET", "/admin/activity?action=admin.", nil, bearer(staff))
	e.must(200, code, body, "activity")
	for _, want := range []string{"admin.user.suspend", "admin.user.delete", "admin.tenant.delete"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("activity lacks %s: %s", want, body)
		}
	}
}
