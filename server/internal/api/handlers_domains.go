package api

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/store"
	"github.com/memrohq/memro/server/internal/workemail"
)

// Self-service organisations and verified company domains (ADR 0027).
// A work email only decides what the dashboard offers. Authority over a
// company comes from a DNS TXT record (or Memro staff), and joining through a
// domain never makes anyone more than what an owner or admin approves.

const (
	maxOrgsPerUser    = 5
	maxDomainsPerOrg  = 10
	domainChallenge   = "_memro-challenge."
	domainTokenPrefix = "memro-verify="
	domainRecheck     = 7 * 24 * time.Hour
)

var hostnameRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])$`)

// normDomain lower-cases a domain and checks it is a hostname a company could
// own, or returns "".
func normDomain(raw string) string {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if len(d) > 253 || !hostnameRe.MatchString(d) || !workemail.IsWorkDomain(d) {
		return ""
	}
	return d
}

// hasToken reports whether one of the TXT records proves the claim.
func hasToken(records []string, token string) bool {
	return slices.ContainsFunc(records, func(r string) bool { return strings.TrimSpace(r) == domainTokenPrefix+token })
}

// checkDomain looks up the challenge record. found is false with a nil error
// when DNS answered without it; err is set when DNS could not answer.
func (s *Server) checkDomain(ctx context.Context, domain, token string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	records, err := s.lookupTXT(ctx, domainChallenge+domain)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hasToken(records, token), nil
}

func domainResponse(d store.TenantDomain) map[string]any {
	res := map[string]any{"domain": d.Domain, "verified": d.VerifiedAt != nil, "verified_at": d.VerifiedAt,
		"checked_at": d.CheckedAt, "created_at": d.CreatedAt}
	if d.Token != nil {
		res["txt_name"] = domainChallenge + d.Domain
		res["txt_value"] = domainTokenPrefix + *d.Token
	}
	return res
}

// sessionUser returns the signed-in person's ID and email, or writes the error.
func (s *Server) sessionUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	a := agentFrom(r)
	if !requireSession(w, a) || a.UserID == nil {
		return uuid.Nil, "", false
	}
	email, err := s.store.UserEmail(r.Context(), *a.UserID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return uuid.Nil, "", false
	}
	return *a.UserID, email, true
}

// GET /orgs/discover — what the signed-in person may do: create an
// organisation (work email), and organisations at their verified domain that
// take join requests.
func (s *Server) discoverOrgs(w http.ResponseWriter, r *http.Request) {
	uid, email, ok := s.sessionUser(w, r)
	if !ok {
		return
	}
	domain := workemail.Domain(email)
	work := workemail.IsWorkDomain(domain)
	orgs := []store.DiscoverableOrg{}
	if work {
		var err error
		if orgs, err = s.store.DiscoverableOrgs(r.Context(), domain, uid); err != nil {
			httpError(w, http.StatusInternalServerError, "Database error")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "domain": domain, "work_email": work, "orgs": orgs})
}

// POST /orgs {"name"} — anyone signed in with a work email may create an
// organisation and owns it. It starts on the trial (ADR 0028), or with it
// already ended when the company (email domain) had one, and holds no domain
// until one is verified.
func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	uid, email, ok := s.sessionUser(w, r)
	if !ok {
		return
	}
	if !workemail.IsWorkEmail(email) {
		httpError(w, http.StatusForbidden, "Sign in with your work email to create an organisation")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(body(r), &req)
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		httpError(w, http.StatusBadRequest, "name is required (at most 100 characters)")
		return
	}
	n, err := s.store.OrgsCreatedBy(r.Context(), uid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if n >= maxOrgsPerUser {
		httpError(w, http.StatusTooManyRequests, "You have created the maximum number of organisations")
		return
	}
	m, trial, err := s.store.CreateOrg(r.Context(), name, uid, workemail.Domain(email), trialLimits())
	if err != nil {
		slog.Error("create org", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create organisation")
		return
	}
	resource := m.TenantID.String()
	if !trial {
		resource += " (trial already used by " + workemail.Domain(email) + ")"
	}
	s.audit(r, agentFrom(r), "org.create", &resource)
	writeJSON(w, http.StatusCreated, map[string]any{"tenant_id": m.TenantID, "name": m.Name, "role": m.Role, "trial_started": trial})
}

// POST /orgs/{id}/join-requests — ask to join an organisation that holds the
// person's email domain verified and takes requests.
func (s *Server) requestToJoin(w http.ResponseWriter, r *http.Request) {
	uid, email, ok := s.sessionUser(w, r)
	if !ok {
		return
	}
	tid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid tenant ID format")
		return
	}
	orgs, err := s.store.DiscoverableOrgs(r.Context(), workemail.Domain(email), uid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	// Same answer for "no such org" and "not for you", so IDs can't be probed.
	if !slices.ContainsFunc(orgs, func(o store.DiscoverableOrg) bool { return o.TenantID == tid }) {
		httpError(w, http.StatusNotFound, "No organisation you can ask to join")
		return
	}
	if err := s.store.CreateJoinRequest(r.Context(), tid, uid); err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := tid.String()
	s.audit(r, agentFrom(r), "join_request.create", &resource)
	w.WriteHeader(http.StatusAccepted)
}

// GET /tenant/{id}/join-requests
func (s *Server) listJoinRequests(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	reqs, err := s.store.JoinRequests(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, reqs)
}

// POST /tenant/{id}/join-requests/{request_id} {"role"} approves with that
// role (default viewer); DELETE declines.
func (s *Server) decideJoinRequest(w http.ResponseWriter, r *http.Request) {
	tid, role, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("request_id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid request ID")
		return
	}
	newRole := "" // declines
	if r.Method == http.MethodPost {
		var req struct {
			Role string `json:"role"`
		}
		_ = decodeJSON(body(r), &req)
		newRole = req.Role
		if newRole == "" {
			newRole = store.RoleViewer
		}
		if !slices.Contains(store.Roles, newRole) {
			httpError(w, http.StatusBadRequest, "role must be one of owner, admin, billing, viewer")
			return
		}
		if !mayGrant(role, newRole) {
			httpError(w, http.StatusForbidden, "Only an owner can make owners")
			return
		}
		if !s.seatsLeft(w, r, tid) {
			return
		}
	}
	user, err := s.store.DecideJoinRequest(r.Context(), tid, id, newRole)
	switch {
	case err != nil:
		slog.Error("decide join request", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
	case user == nil:
		httpError(w, http.StatusNotFound, "Request not found")
	default:
		action, resource := "join_request.decline", user.String()+" in "+tid.String()
		if newRole != "" {
			action, resource = "join_request.approve", resource+" as "+newRole
		}
		s.audit(r, agentFrom(r), action, &resource)
		w.WriteHeader(http.StatusNoContent)
	}
}

// PATCH /tenant/{id} {"join_policy": "invite" | "request"}
func (s *Server) updateOrg(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		JoinPolicy string `json:"join_policy"`
	}
	_ = decodeJSON(body(r), &req)
	if req.JoinPolicy != store.JoinByInvite && req.JoinPolicy != store.JoinByRequest {
		httpError(w, http.StatusBadRequest, "join_policy must be invite or request")
		return
	}
	if req.JoinPolicy == store.JoinByRequest {
		verified, err := s.store.HasVerifiedDomain(r.Context(), tid)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !verified {
			httpError(w, http.StatusConflict, "Verify a domain first: only people at a verified domain can ask to join")
			return
		}
	}
	if err := s.store.SetJoinPolicy(r.Context(), tid, req.JoinPolicy); err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := tid.String() + " -> " + req.JoinPolicy
	s.audit(r, agentFrom(r), "org.join_policy", &resource)
	writeJSON(w, http.StatusOK, map[string]string{"join_policy": req.JoinPolicy})
}

// GET /tenant/{id}/domains — claims with their TXT records, and the join policy.
func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	domains, err := s.store.TenantDomains(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	policy, err := s.store.JoinPolicy(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	out := make([]map[string]any, len(domains))
	for i, d := range domains {
		out[i] = domainResponse(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": out, "join_policy": policy})
}

// POST /tenant/{id}/domains {"domain"} — claims a domain and returns the TXT
// record that proves it.
func (s *Server) claimDomain(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	_ = decodeJSON(body(r), &req)
	domain := normDomain(req.Domain)
	if domain == "" {
		httpError(w, http.StatusBadRequest, "Enter your company's domain, e.g. example.com (free email providers can't be claimed)")
		return
	}
	n, err := s.store.CountTenantDomains(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if n >= maxDomainsPerOrg {
		httpError(w, http.StatusTooManyRequests, "This organisation has claimed the maximum number of domains")
		return
	}
	d, err := s.store.ClaimDomain(r.Context(), tid, domain, rand.Text())
	if err != nil {
		slog.Error("claim domain", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := domain + " for " + tid.String()
	s.audit(r, agentFrom(r), "tenant_domain.claim", &resource)
	writeJSON(w, http.StatusCreated, domainResponse(d))
}

// POST /tenant/{id}/domains/{domain}/verify — checks the TXT record now.
func (s *Server) verifyDomain(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	d, err := s.store.TenantDomainClaim(r.Context(), tid, strings.ToLower(r.PathValue("domain")))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if d == nil {
		httpError(w, http.StatusNotFound, "Domain not claimed by this organisation")
		return
	}
	if d.Token == nil { // verified by Memro staff
		writeJSON(w, http.StatusOK, domainResponse(*d))
		return
	}
	found, err := s.checkDomain(r.Context(), d.Domain, *d.Token)
	if err != nil {
		slog.Warn("domain check", "domain", d.Domain, "err", err)
		httpError(w, http.StatusBadGateway, "DNS lookup failed; try again in a minute")
		return
	}
	if !found {
		httpError(w, http.StatusUnprocessableEntity, "TXT record not found yet. DNS changes can take up to an hour to appear.")
		return
	}
	switch err := s.store.SetDomainVerified(r.Context(), tid, d.Domain, true); {
	case errors.Is(err, store.ErrDomainTaken):
		httpError(w, http.StatusConflict, "Another organisation already verified this domain. Contact support if it's yours.")
		return
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := d.Domain + " for " + tid.String()
	s.audit(r, agentFrom(r), "tenant_domain.verify", &resource)
	if d, err = s.store.TenantDomainClaim(r.Context(), tid, d.Domain); err != nil || d == nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, domainResponse(*d))
}

// DELETE /tenant/{id}/domains/{domain}
func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	domain := strings.ToLower(r.PathValue("domain"))
	found, err := s.store.DeleteTenantDomain(r.Context(), tid, domain)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Database error")
	case !found:
		httpError(w, http.StatusNotFound, "Domain not claimed by this organisation")
	default:
		resource := domain + " for " + tid.String()
		s.audit(r, agentFrom(r), "tenant_domain.delete", &resource)
		w.WriteHeader(http.StatusNoContent)
	}
}

// recheckDomains re-checks DNS-verified domains last checked a week ago: a
// domain whose record is gone loses its verification (companies change
// hands). A DNS failure leaves it as it was. Run by RunHourlyJobs.
func (s *Server) recheckDomains(ctx context.Context) {
	due, err := s.store.DomainsToRecheck(ctx, time.Now().Add(-domainRecheck), 100)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("domain recheck", "err", err)
		}
		return
	}
	for _, d := range due {
		found, err := s.checkDomain(ctx, d.Domain, d.Token)
		if err != nil {
			slog.Warn("domain recheck lookup", "domain", d.Domain, "err", err)
			continue
		}
		if err := s.store.SetDomainVerified(ctx, d.TenantID, d.Domain, found); err != nil {
			slog.Warn("domain recheck save", "domain", d.Domain, "err", err)
		} else if !found {
			slog.Warn("domain verification lost", "domain", d.Domain, "tenant", d.TenantID)
		}
	}
}
