package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/memrohq/memro/server/internal/api"
)

// fakeDNS serves TXT records from a map; a missing name is NXDOMAIN.
type fakeDNS struct {
	mu      sync.Mutex
	records map[string][]string
}

func (f *fakeDNS) lookup(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.records[name]; ok {
		return r, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *fakeDNS) set(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value == "" {
		delete(f.records, name)
	} else {
		f.records[name] = []string{"v=spf1 -all", value}
	}
}

// ADR 0027: a work email may create an organisation, but only a DNS record
// proves the company; then colleagues may ask to join and an owner decides.
func TestOrgDomainVerificationAndJoinRequests(t *testing.T) {
	dns := &fakeDNS{records: map[string][]string{}}
	var deps *api.Deps
	e := setup(t, func(d *api.Deps) { d.LookupTXT = dns.lookup; deps = d })
	company := fmt.Sprintf("acme%d.test", time.Now().UnixNano())
	at := func(prefix string) string { return fmt.Sprintf("%s-%d@%s", prefix, time.Now().UnixNano(), company) }

	// A free mailbox can't create an organisation.
	_, gmail := e.dashboardSession(fmt.Sprintf("me%d@gmail.com", time.Now().UnixNano()))
	code, body := e.do(nil, "", "GET", "/orgs/discover", nil, bearer(gmail))
	e.must(200, code, body, "discover (gmail)")
	if !strings.Contains(string(body), `"work_email":false`) {
		t.Fatalf("gmail discover = %s", body)
	}
	code, body = e.do(nil, "", "POST", "/orgs", j(map[string]string{"name": "Mine"}), bearer(gmail))
	e.must(403, code, body, "create org with gmail")

	// A work email creates one and owns it, on the trial (ADR 0028).
	_, owner := e.dashboardSession(at("owner"))
	code, body = e.do(nil, "", "POST", "/orgs", j(map[string]string{"name": "Acme"}), bearer(owner))
	e.must(201, code, body, "create org")
	var org struct {
		TenantID string `json:"tenant_id"`
		Role     string
	}
	_ = json.Unmarshal(body, &org)
	if org.Role != "owner" {
		t.Fatalf("creator role = %q", org.Role)
	}
	base := "/tenant/" + org.TenantID
	code, body = e.do(nil, "", "GET", base+"/limits", nil, bearer(owner))
	e.must(200, code, body, "trial limits")
	if !strings.Contains(string(body), `"plan":"trial"`) || !strings.Contains(string(body), `"max_stored_tokens_total":100000`) {
		t.Fatalf("new org limits = %s", body)
	}
	code, body = e.do(nil, "", "PATCH", base, j(map[string]string{"join_policy": "request"}), bearer(owner))
	e.must(409, code, body, "join requests before verifying")

	// Claim: free providers are refused; a company domain gets a TXT record.
	code, body = e.do(nil, "", "POST", base+"/domains", j(map[string]string{"domain": "gmail.com"}), bearer(owner))
	e.must(400, code, body, "claim gmail.com")
	code, body = e.do(nil, "", "POST", base+"/domains", j(map[string]string{"domain": strings.ToUpper(company) + "."}), bearer(owner))
	e.must(201, code, body, "claim domain")
	var claim struct {
		Domain   string
		Verified bool
		TxtName  string `json:"txt_name"`
		TxtValue string `json:"txt_value"`
	}
	_ = json.Unmarshal(body, &claim)
	if claim.Domain != company || claim.Verified || claim.TxtName != "_memro-challenge."+company || !strings.HasPrefix(claim.TxtValue, "memro-verify=") {
		t.Fatalf("claim = %s", body)
	}

	// A rival org claims the same domain; claiming proves nothing.
	_, rival := e.dashboardSession(at("rival"))
	code, body = e.do(nil, "", "POST", "/orgs", j(map[string]string{"name": "Not Acme"}), bearer(rival))
	e.must(201, code, body, "rival org")
	var rivalOrg struct {
		TenantID string `json:"tenant_id"`
	}
	_ = json.Unmarshal(body, &rivalOrg)
	code, body = e.do(nil, "", "POST", "/tenant/"+rivalOrg.TenantID+"/domains", j(map[string]string{"domain": company}), bearer(rival))
	e.must(201, code, body, "rival claim")

	verify := base + "/domains/" + company + "/verify"
	code, body = e.do(nil, "", "POST", verify, nil, bearer(owner))
	e.must(422, code, body, "verify before the record exists")
	dns.set(claim.TxtName, claim.TxtValue)
	code, body = e.do(nil, "", "POST", verify, nil, bearer(owner))
	e.must(200, code, body, "verify")
	if !strings.Contains(string(body), `"verified":true`) {
		t.Fatalf("verify = %s", body)
	}

	// The rival can't verify with the owner's record, nor with its own once
	// the domain is held.
	code, body = e.do(nil, "", "POST", "/tenant/"+rivalOrg.TenantID+"/domains/"+company+"/verify", nil, bearer(rival))
	e.must(422, code, body, "rival verify with someone else's record")

	// Colleagues may ask to join once the owner allows it.
	_, colleague := e.dashboardSession(at("colleague"))
	code, body = e.do(nil, "", "POST", "/orgs/"+org.TenantID+"/join-requests", nil, bearer(colleague))
	e.must(404, code, body, "ask to join an invite-only org")
	code, body = e.do(nil, "", "PATCH", base, j(map[string]string{"join_policy": "request"}), bearer(owner))
	e.must(200, code, body, "allow join requests")
	code, body = e.do(nil, "", "GET", "/orgs/discover", nil, bearer(colleague))
	e.must(200, code, body, "colleague discover")
	if !strings.Contains(string(body), org.TenantID) || strings.Contains(string(body), rivalOrg.TenantID) {
		t.Fatalf("colleague discover = %s", body)
	}
	code, body = e.do(nil, "", "POST", "/orgs/"+org.TenantID+"/join-requests", nil, bearer(colleague))
	e.must(202, code, body, "ask to join")
	code, body = e.do(nil, "", "POST", "/orgs/"+org.TenantID+"/join-requests", nil, bearer(gmail))
	e.must(404, code, body, "outsider asks to join")

	// Asking grants nothing until an owner approves, and only the role given.
	code, body = e.do(nil, "", "GET", base+"/members", nil, bearer(colleague))
	e.must(403, code, body, "requester reads members")
	code, body = e.do(nil, "", "GET", base+"/join-requests", nil, bearer(owner))
	e.must(200, code, body, "list join requests")
	var reqs []struct{ ID, Email string }
	_ = json.Unmarshal(body, &reqs)
	if len(reqs) != 1 || !strings.HasPrefix(reqs[0].Email, "colleague-") {
		t.Fatalf("join requests = %s", body)
	}
	code, body = e.do(nil, "", "POST", base+"/join-requests/"+reqs[0].ID, j(map[string]string{}), bearer(owner))
	e.must(204, code, body, "approve")
	code, body = e.do(nil, "", "GET", "/orgs", nil, bearer(colleague))
	e.must(200, code, body, "colleague orgs")
	if !strings.Contains(string(body), `"role":"viewer"`) {
		t.Fatalf("approved colleague orgs = %s", body)
	}

	// The weekly re-check drops a domain whose record is gone.
	dns.set(claim.TxtName, "")
	if _, err := e.st.Pool.Exec(context.Background(), `UPDATE tenant_domains SET checked_at = NOW() - interval '8 days' WHERE domain = $1`, company); err != nil {
		t.Fatal(err)
	}
	api.RecheckDomains(api.NewServer(*deps), context.Background())
	code, body = e.do(nil, "", "GET", base+"/domains", nil, bearer(owner))
	e.must(200, code, body, "domains after recheck")
	if !strings.Contains(string(body), `"verified":false`) {
		t.Fatalf("domain still verified after its record was removed: %s", body)
	}
}
