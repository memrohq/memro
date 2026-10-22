package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

type claimResponse struct {
	Granted        bool       `json:"granted"`
	LeaseID        *uuid.UUID `json:"lease_id,omitempty"`
	Holder         string     `json:"holder,omitempty"`
	HolderSession  string     `json:"holder_session,omitempty"`
	HolderResource string     `json:"holder_resource,omitempty"`
	Intent         string     `json:"intent,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	HandoffSummary string     `json:"handoff_summary,omitempty"`
	HandoffFrom    string     `json:"handoff_from,omitempty"`
}

type checkResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type heartbeatResponse struct {
	Renewed   bool      `json:"renewed"`
	ExpiresAt time.Time `json:"expires_at"`
}

type activeResponse struct {
	Leases  []store.Lease  `json:"leases"`
	Intents []store.Intent `json:"intents"`
}

// TestCoordination_ConcurrentClaimsRace100x runs 100 consecutive races between two
// concurrent goroutines claiming the exact same resource. In every race, exactly one wins.
func TestCoordination_ConcurrentClaimsRace100x(t *testing.T) {
	e := setup(t)

	// Two agents owned by the same user to share the same coordination scope
	email := fmt.Sprintf("race-owner-%d@example.com", time.Now().UnixNano())
	ctx := context.Background()
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		resource := fmt.Sprintf("race/resource-%d.go", i)
		var wg sync.WaitGroup
		wg.Add(2)

		var respA, respB claimResponse
		var statusA, statusB int

		go func() {
			defer wg.Done()
			req := map[string]any{
				"resource": resource,
				"intent":   "race from A",
			}
			code, b := e.do(&agentA, "v2", "POST", "/coordination/claim", j(req), nil)
			statusA = code
			_ = json.Unmarshal(b, &respA)
		}()

		go func() {
			defer wg.Done()
			req := map[string]any{
				"resource": resource,
				"intent":   "race from B",
			}
			code, b := e.do(&agentB, "v2", "POST", "/coordination/claim", j(req), nil)
			statusB = code
			_ = json.Unmarshal(b, &respB)
		}()

		wg.Wait()

		if statusA != 200 || statusB != 200 {
			t.Fatalf("iteration %d: expected 200 OK for both, got A=%d, B=%d", i, statusA, statusB)
		}

		// Exactly one must be granted, and the other denied
		if respA.Granted == respB.Granted {
			t.Fatalf("iteration %d: conflict! both had granted=%v (A: %+v, B: %+v)", i, respA.Granted, respA, respB)
		}

		if respA.Granted {
			if respB.Holder != agentA.id {
				t.Fatalf("iteration %d: B should show holder=agentA, got %q", i, respB.Holder)
			}
		} else {
			if respA.Holder != agentB.id {
				t.Fatalf("iteration %d: A should show holder=agentB, got %q", i, respA.Holder)
			}
		}
	}
}

// TestCoordination_ExpiryAndReap tests that an expired lease is automatically reaped
// and allows another agent to claim the resource.
func TestCoordination_ExpiryAndReap(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	email := fmt.Sprintf("expiry-owner-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	resource := "cache/redis.go"

	// 1. Agent A claims with 1 second TTL
	code, b := e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource":    resource,
		"intent":      "flush expired keys",
		"ttl_seconds": 1,
	}), nil)
	e.must(200, code, b, "agent A claim with short TTL")

	var claimA claimResponse
	_ = json.Unmarshal(b, &claimA)
	if !claimA.Granted {
		t.Fatalf("agent A claim not granted: %+v", claimA)
	}

	// 2. Fast forward time by updating expires_at to the past
	scopeKey := store.CoordinationScopeKey(uuid.Nil, &u.ID, agentA.id)
	_, err = e.st.Pool.Exec(ctx, `UPDATE coordination_leases 
		SET expires_at = NOW() - interval '2 seconds' 
		WHERE scope_key = $1 AND resource = $2`, scopeKey, resource)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Agent B checks: should allow because previous lease expired
	code, b = e.do(&agentB, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
	}), nil)
	e.must(200, code, b, "agent B check expired resource")
	var checkB checkResponse
	_ = json.Unmarshal(b, &checkB)
	if checkB.Decision != "allow" {
		t.Fatalf("expected decision allow for expired lease, got %s: %s", checkB.Decision, checkB.Reason)
	}

	// 4. Agent B claims: reaps the expired lease and succeeds
	code, b = e.do(&agentB, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "new work",
	}), nil)
	e.must(200, code, b, "agent B claim after expiry")

	var claimB claimResponse
	_ = json.Unmarshal(b, &claimB)
	if !claimB.Granted {
		t.Fatalf("agent B claim should be granted after expiry: %+v", claimB)
	}

	// 5. Verify the old lease is marked with release_status = 'expired'
	var expiredCount int
	err = e.st.Pool.QueryRow(ctx, `SELECT count(*) FROM coordination_leases 
		WHERE scope_key = $1 AND resource = $2 AND release_status = 'expired'`, scopeKey, resource).Scan(&expiredCount)
	if err != nil {
		t.Fatal(err)
	}
	if expiredCount < 1 {
		t.Fatalf("expected at least 1 expired lease in table, got %d", expiredCount)
	}
}

// TestCoordination_HeartbeatAndReclaim tests extending leases via heartbeat and idempotent reclaim.
func TestCoordination_HeartbeatAndReclaim(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	email := fmt.Sprintf("hb-owner-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	resource := "worker/cron.go"

	// 1. Agent A claims
	code, b := e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource":    resource,
		"intent":      "initial work",
		"ttl_seconds": 60,
	}), nil)
	e.must(200, code, b, "claim initial")
	var claim1 claimResponse
	_ = json.Unmarshal(b, &claim1)

	// 2. Non-holder (Agent B) attempts heartbeat -> fails with 403
	code, b = e.do(&agentB, "v2", "POST", "/coordination/heartbeat", j(map[string]any{
		"resource": resource,
	}), nil)
	if code == 200 {
		t.Fatalf("non-holder heartbeat should fail, got 200: %s", b)
	}

	// 3. Holder (Agent A) heartbeats -> extends expires_at
	code, b = e.do(&agentA, "v2", "POST", "/coordination/heartbeat", j(map[string]any{
		"resource":    resource,
		"ttl_seconds": 300,
	}), nil)
	e.must(200, code, b, "holder heartbeat")
	var hbResp heartbeatResponse
	_ = json.Unmarshal(b, &hbResp)
	if !hbResp.Renewed {
		t.Fatalf("expected renewed: true")
	}
	if !hbResp.ExpiresAt.After(*claim1.ExpiresAt) {
		t.Fatalf("heartbeat should extend expires_at: initial %v, renewed %v", claim1.ExpiresAt, hbResp.ExpiresAt)
	}

	// 4. Re-claiming your own lease is idempotent and extends it
	code, b = e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource":    resource,
		"intent":      "updated intent",
		"ttl_seconds": 600,
	}), nil)
	e.must(200, code, b, "reclaim own lease")
	var reclaimResp claimResponse
	_ = json.Unmarshal(b, &reclaimResp)
	if !reclaimResp.Granted {
		t.Fatalf("reclaiming own lease should be granted: %+v", reclaimResp)
	}
	if *reclaimResp.LeaseID != *claim1.LeaseID {
		t.Fatalf("reclaiming should preserve lease ID: original %v, got %v", claim1.LeaseID, reclaimResp.LeaseID)
	}
}

// TestCoordination_ReleaseAndHandoff tests release protections and handoff summary forwarding.
func TestCoordination_ReleaseAndHandoff(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	email := fmt.Sprintf("handoff-owner-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	resource := "feature/auth.go"

	// 1. Agent A claims
	code, b := e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "rewrite auth middleware",
	}), nil)
	e.must(200, code, b, "claim auth.go")

	// 2. Agent B tries to release -> rejected
	code, b = e.do(&agentB, "v2", "POST", "/coordination/release", j(map[string]any{
		"resource": resource,
	}), nil)
	if code == 200 {
		t.Fatalf("non-holder release must fail, got 200: %s", b)
	}

	// 3. Agent B tries to handoff -> rejected
	code, b = e.do(&agentB, "v2", "POST", "/coordination/handoff", j(map[string]any{
		"resource": resource,
		"summary":  "malicious handoff",
	}), nil)
	if code == 200 {
		t.Fatalf("non-holder handoff must fail, got 200: %s", b)
	}

	// 4. Agent A hands off to Agent B
	code, b = e.do(&agentA, "v2", "POST", "/coordination/handoff", j(map[string]any{
		"resource":      resource,
		"summary":       "Refactored JWT parsing; please write unit tests.",
		"next_agent_id": agentB.id,
	}), nil)
	e.must(200, code, b, "agent A handoff")

	// 5. Agent B checks -> allowed
	code, b = e.do(&agentB, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
	}), nil)
	e.must(200, code, b, "check after handoff")
	var chk checkResponse
	_ = json.Unmarshal(b, &chk)
	if chk.Decision != "allow" {
		t.Fatalf("expected allow after handoff, got %s", chk.Decision)
	}

	// 6. Agent B claims -> granted, and sees handoff summary
	code, b = e.do(&agentB, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "writing unit tests",
	}), nil)
	e.must(200, code, b, "agent B claim after handoff")
	var claimB claimResponse
	_ = json.Unmarshal(b, &claimB)
	if !claimB.Granted {
		t.Fatalf("agent B claim should be granted: %+v", claimB)
	}
	if claimB.HandoffSummary != "Refactored JWT parsing; please write unit tests." {
		t.Fatalf("missing or wrong handoff summary: %q", claimB.HandoffSummary)
	}
	if claimB.HandoffFrom != agentA.id {
		t.Fatalf("wrong handoff from: %q, want %q", claimB.HandoffFrom, agentA.id)
	}

	// 7. Agent B releases with completed
	code, b = e.do(&agentB, "v2", "POST", "/coordination/release", j(map[string]any{
		"resource": resource,
		"status":   "completed",
	}), nil)
	e.must(200, code, b, "agent B release")

	// 8. Active list shows no leases
	code, b = e.do(&agentB, "v2", "GET", "/coordination/active?resource="+resource, nil, nil)
	e.must(200, code, b, "get active")
	var act activeResponse
	_ = json.Unmarshal(b, &act)
	if len(act.Leases) != 0 {
		t.Fatalf("expected 0 active leases after release, got %d", len(act.Leases))
	}
}

// TestCoordination_ScopeIsolation tests strict isolation across:
// - Tenant A vs Tenant B
// - Two agents owned by User 1 (share scope)
// - User 1 vs User 2 (isolated)
// - Unclaimed agents (isolated per agent)
func TestCoordination_ScopeIsolation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// ── 1. Tenant A vs Tenant B ──────────────────────────────────────────────
	tidA := uuid.New()
	tidB := uuid.New()
	if err := e.st.CreateTenant(ctx, &domain.Tenant{ID: tidA, Name: "Tenant A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateTenant(ctx, &domain.Tenant{ID: tidB, Name: "Tenant B"}); err != nil {
		t.Fatal(err)
	}

	agentTA := newAgent(t)
	agentTB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentTA.id, nil, tidA); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentTB.id, nil, tidB); err != nil {
		t.Fatal(err)
	}

	sharedResource := "deploy/k8s.yaml"

	// Agent TA claims in Tenant A
	code, b := e.do(&agentTA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": sharedResource,
		"intent":   "scaling cluster A",
	}), nil)
	e.must(200, code, b, "tenant A claim")
	var cTA claimResponse
	_ = json.Unmarshal(b, &cTA)
	if !cTA.Granted {
		t.Fatalf("tenant A claim should be granted: %+v", cTA)
	}

	// Agent TB claims same resource in Tenant B -> MUST SUCCEED (Tenant isolation!)
	code, b = e.do(&agentTB, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": sharedResource,
		"intent":   "scaling cluster B",
	}), nil)
	e.must(200, code, b, "tenant B claim")
	var cTB claimResponse
	_ = json.Unmarshal(b, &cTB)
	if !cTB.Granted {
		t.Fatalf("tenant B should NOT be blocked by tenant A claim: %+v", cTB)
	}

	// ── 2. Two agents owned by User 1 share scope ────────────────────────────
	email1 := fmt.Sprintf("u1-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email1, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u1, _ := e.st.GetUserByEmail(ctx, email1, uuid.Nil)

	agentU1_A := newAgent(t)
	agentU1_B := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentU1_A.id, &u1.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentU1_B.id, &u1.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	userResource := "billing/stripe.go"

	// Agent 1A claims
	code, b = e.do(&agentU1_A, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": userResource,
		"intent":   "upgrade stripe api",
	}), nil)
	e.must(200, code, b, "user1 agent A claim")

	// Agent 1B checks -> MUST BLOCK
	code, b = e.do(&agentU1_B, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": userResource,
	}), nil)
	e.must(200, code, b, "user1 agent B check")
	var chk1B checkResponse
	_ = json.Unmarshal(b, &chk1B)
	if chk1B.Decision != "block" {
		t.Fatalf("user1 agent B should be BLOCKED by agent A's lease, got %s", chk1B.Decision)
	}

	// Agent 1B claims -> MUST BE DENIED
	code, b = e.do(&agentU1_B, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": userResource,
		"intent":   "conflicting claim",
	}), nil)
	e.must(200, code, b, "user1 agent B claim")
	var c1B claimResponse
	_ = json.Unmarshal(b, &c1B)
	if c1B.Granted {
		t.Fatalf("user1 agent B should be denied claim: %+v", c1B)
	}

	// ── 3. User 1 vs User 2 are isolated ─────────────────────────────────────
	email2 := fmt.Sprintf("u2-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email2, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u2, _ := e.st.GetUserByEmail(ctx, email2, uuid.Nil)

	agentU2 := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentU2.id, &u2.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	// Agent U2 claims same resource -> MUST SUCCEED (User isolation!)
	code, b = e.do(&agentU2, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": userResource,
		"intent":   "independent user 2 work",
	}), nil)
	e.must(200, code, b, "user 2 claim")
	var cU2 claimResponse
	_ = json.Unmarshal(b, &cU2)
	if !cU2.Granted {
		t.Fatalf("user 2 should not be blocked by user 1: %+v", cU2)
	}

	// ── 4. Unclaimed agents see only their own leases ─────────────────────────
	agentUnclaimed1 := newAgent(t)
	agentUnclaimed2 := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentUnclaimed1.id, nil, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentUnclaimed2.id, nil, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	unclaimedResource := "local/scratch.py"

	// Unclaimed 1 claims
	code, b = e.do(&agentUnclaimed1, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": unclaimedResource,
		"intent":   "quick script",
	}), nil)
	e.must(200, code, b, "unclaimed 1 claim")

	// Unclaimed 2 claims same resource -> MUST SUCCEED (separate agent scopes)
	code, b = e.do(&agentUnclaimed2, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": unclaimedResource,
		"intent":   "another script",
	}), nil)
	e.must(200, code, b, "unclaimed 2 claim")
	var cUncl2 claimResponse
	_ = json.Unmarshal(b, &cUncl2)
	if !cUncl2.Granted {
		t.Fatalf("unclaimed 2 should not be blocked by unclaimed 1: %+v", cUncl2)
	}
}

// TestCoordination_CheckDecisions tests allow | warn | block logic.
func TestCoordination_CheckDecisions(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	email := fmt.Sprintf("check-owner-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	resource := "services/order.go"

	// 1. Initial state: unvisited -> allow
	code, b := e.do(&agentA, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "initial inspection",
	}), nil)
	e.must(200, code, b, "initial check")
	var chk1 checkResponse
	_ = json.Unmarshal(b, &chk1)
	if chk1.Decision != "allow" {
		t.Fatalf("expected allow for untouched resource, got %s", chk1.Decision)
	}

	// 2. Agent A logs intent
	code, b = e.do(&agentA, "v2", "POST", "/coordination/intent", j(map[string]any{
		"resource": resource,
		"intent":   "planning refactor of checkout flow",
	}), nil)
	e.must(200, code, b, "log intent")

	// 3. Agent B checks -> warn (active intent by another agent)
	code, b = e.do(&agentB, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "touching checkout",
	}), nil)
	e.must(200, code, b, "check with intent")
	var chk2 checkResponse
	_ = json.Unmarshal(b, &chk2)
	if chk2.Decision != "warn" {
		t.Fatalf("expected warn due to Agent A's intent, got %s: %s", chk2.Decision, chk2.Reason)
	}

	// 4. Agent A checks -> allow (own intent does not warn yourself)
	code, b = e.do(&agentA, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "checking my own flow",
	}), nil)
	e.must(200, code, b, "own intent check")
	var chkSelf checkResponse
	_ = json.Unmarshal(b, &chkSelf)
	if chkSelf.Decision != "allow" {
		t.Fatalf("expected allow for own intent, got %s", chkSelf.Decision)
	}

	// 5. Agent A claims the resource
	code, b = e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "refactoring checkout flow",
	}), nil)
	e.must(200, code, b, "agent A claim")

	// 6. Agent B checks -> block (hard lease held by another)
	code, b = e.do(&agentB, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "trying to work on checkout",
	}), nil)
	e.must(200, code, b, "check with active lease")
	var chk3 checkResponse
	_ = json.Unmarshal(b, &chk3)
	if chk3.Decision != "block" {
		t.Fatalf("expected block due to Agent A's lease, got %s", chk3.Decision)
	}

	// 7. Agent A checks -> allow (own lease allows)
	code, b = e.do(&agentA, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "continue my work",
	}), nil)
	e.must(200, code, b, "own lease check")
	var chkOwnLease checkResponse
	_ = json.Unmarshal(b, &chkOwnLease)
	if chkOwnLease.Decision != "allow" {
		t.Fatalf("expected allow for own lease, got %s", chkOwnLease.Decision)
	}
}

// TestCoordination_EndToEnd_TwoAgentsSameUserHandoff tests:
// two separately registered agents owned by the same user:
// agent A claims, agent B's pre_action_check blocks, A hands off, B claims and sees the summary.
func TestCoordination_EndToEnd_TwoAgentsSameUserHandoff(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// 1. Create a user
	email := fmt.Sprintf("e2e-owner-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}

	// 2. Register Agent A and Agent B separately, both owned by the same user
	agentA := newAgent(t)
	agentB := newAgent(t)
	if err := e.st.CreateIdentity(ctx, agentA.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateIdentity(ctx, agentB.id, &u.ID, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	resource := "src/worker/pipeline.go"

	// 3. Agent A claims the resource
	code, b := e.do(&agentA, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "refactoring pipeline processing",
	}), nil)
	e.must(200, code, b, "agent A claim")
	var claimA claimResponse
	_ = json.Unmarshal(b, &claimA)
	if !claimA.Granted {
		t.Fatalf("agent A claim not granted: %+v", claimA)
	}

	// 4. Agent B's pre_action_check blocks
	code, b = e.do(&agentB, "v2", "POST", "/coordination/check", j(map[string]any{
		"resource": resource,
		"intent":   "modify pipeline",
	}), nil)
	e.must(200, code, b, "agent B check")
	var checkB checkResponse
	_ = json.Unmarshal(b, &checkB)
	if checkB.Decision != "block" {
		t.Fatalf("agent B pre_action_check should block, got %s: %s", checkB.Decision, checkB.Reason)
	}

	// 5. Agent A hands off work with a summary
	summaryText := "Refactored pipeline stages; ready for benchmark and profiling."
	code, b = e.do(&agentA, "v2", "POST", "/coordination/handoff", j(map[string]any{
		"resource":      resource,
		"summary":       summaryText,
		"next_agent_id": agentB.id,
	}), nil)
	e.must(200, code, b, "agent A handoff")

	// 6. Agent B claims and sees the summary
	code, b = e.do(&agentB, "v2", "POST", "/coordination/claim", j(map[string]any{
		"resource": resource,
		"intent":   "run benchmarks and profiling",
	}), nil)
	e.must(200, code, b, "agent B claim after handoff")
	var claimB claimResponse
	_ = json.Unmarshal(b, &claimB)
	if !claimB.Granted {
		t.Fatalf("agent B claim should be granted after handoff: %+v", claimB)
	}
	if claimB.HandoffSummary != summaryText {
		t.Fatalf("expected handoff summary %q, got %q", summaryText, claimB.HandoffSummary)
	}
	if claimB.HandoffFrom != agentA.id {
		t.Fatalf("expected handoff from %q, got %q", agentA.id, claimB.HandoffFrom)
	}
}

// Two tools on one account share an agent but are different sessions (the MCP
// server sends each connection's Mcp-Session-Id). They must block each other.
func TestCoordination_SameAgentDifferentSessionsBlock(t *testing.T) {
	e := setup(t)
	agent := newAgent(t)
	if err := e.st.CreateIdentity(context.Background(), agent.id, nil, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	res := "src/shared.go"
	n := 0
	claim := func(session string) claimResponse {
		n++ // identical signed bodies would trip the replay cache
		code, b := e.do(&agent, "v2", "POST", "/coordination/claim", j(map[string]any{"resource": res, "intent": fmt.Sprintf("edit %d", n), "session_id": session}), nil)
		e.must(200, code, b, "claim as "+session)
		var r claimResponse
		_ = json.Unmarshal(b, &r)
		return r
	}
	if r := claim("mcp:claude-code"); !r.Granted {
		t.Fatalf("first session should be granted: %+v", r)
	}
	if r := claim("mcp:cursor"); r.Granted || r.HolderSession != "mcp:claude-code" {
		t.Fatalf("another session of the same agent must be denied: %+v", r)
	}
	code, b := e.do(&agent, "v2", "POST", "/coordination/check", j(map[string]any{"resource": res, "session_id": "mcp:cursor"}), nil)
	var c checkResponse
	_ = json.Unmarshal(b, &c)
	if code != 200 || c.Decision != "block" {
		t.Fatalf("check from the other session = %d %s, want block", code, b)
	}
	code, b = e.do(&agent, "v2", "POST", "/coordination/check", j(map[string]any{"resource": res, "session_id": "mcp:claude-code"}), nil)
	_ = json.Unmarshal(b, &c)
	if code != 200 || c.Decision != "allow" {
		t.Fatalf("check from the holder = %d %s, want allow", code, b)
	}
	// The other session can't release or hand off by resource name.
	code, _ = e.do(&agent, "v2", "POST", "/coordination/release", j(map[string]any{"resource": res, "session_id": "mcp:cursor"}), nil)
	if code == 200 {
		t.Fatal("another session released the holder's lease")
	}
	code, _ = e.do(&agent, "v2", "POST", "/coordination/handoff", j(map[string]any{"resource": res, "summary": "x", "session_id": "mcp:cursor"}), nil)
	if code != 409 {
		t.Fatalf("handoff from another session = %d, want 409", code)
	}
	// The holder re-claiming extends its own lease.
	if r := claim("mcp:claude-code"); !r.Granted {
		t.Fatalf("holder re-claim should extend: %+v", r)
	}
	code, b = e.do(&agent, "v2", "POST", "/coordination/release", j(map[string]any{"resource": res, "session_id": "mcp:claude-code"}), nil)
	e.must(200, code, b, "holder release")
	if r := claim("mcp:cursor"); !r.Granted {
		t.Fatalf("after release the other session should be granted: %+v", r)
	}
}

// A claim on a folder covers the files in it, and a claim on a file blocks
// claiming its folder; unrelated paths with a shared prefix and URLs don't
// overlap.
func TestCoordination_FolderAndFileClaimsOverlap(t *testing.T) {
	e := setup(t)
	agent := newAgent(t)
	if err := e.st.CreateIdentity(context.Background(), agent.id, nil, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	n := 0
	claim := func(res, session string) claimResponse {
		n++
		code, b := e.do(&agent, "v2", "POST", "/coordination/claim", j(map[string]any{"resource": res, "intent": fmt.Sprintf("edit %d", n), "session_id": session}), nil)
		e.must(200, code, b, "claim "+res+" as "+session)
		var r claimResponse
		_ = json.Unmarshal(b, &r)
		return r
	}
	check := func(res, session string) checkResponse {
		n++
		code, b := e.do(&agent, "v2", "POST", "/coordination/check", j(map[string]any{"resource": res, "intent": fmt.Sprintf("check %d", n), "session_id": session}), nil)
		e.must(200, code, b, "check "+res+" as "+session)
		var c checkResponse
		_ = json.Unmarshal(b, &c)
		return c
	}

	if r := claim("memro/bench", "claude-code"); !r.Granted {
		t.Fatalf("folder claim should be granted: %+v", r)
	}
	if r := claim("memro/bench/longmemeval/run.py", "antigravity"); r.Granted || r.HolderResource != "memro/bench" || r.HolderSession != "claude-code" {
		t.Fatalf("a file inside a claimed folder must be denied, naming the folder: %+v", r)
	}
	if c := check("./memro/bench//longmemeval/run.py", "antigravity"); c.Decision != "block" {
		t.Fatalf("check on a file inside a claimed folder = %+v, want block", c)
	}
	if c := check("memro/bench/longmemeval/run.py", "claude-code"); c.Decision != "allow" {
		t.Fatalf("the folder's holder may edit inside it: %+v", c)
	}
	if r := claim("memro/bench/longmemeval/run.py", "claude-code"); !r.Granted {
		t.Fatalf("the folder's holder may claim a file inside it: %+v", r)
	}
	if r := claim("memro", "antigravity"); r.Granted {
		t.Fatalf("a parent of a claimed path must be denied: %+v", r)
	}
	if r := claim("memro/benchmarks", "antigravity"); !r.Granted {
		t.Fatalf("a sibling sharing a prefix is not inside the folder: %+v", r)
	}
	if r := claim("https://example.com/a", "claude-code"); !r.Granted {
		t.Fatalf("url claim: %+v", r)
	}
	if r := claim("https://example.com/a/b", "antigravity"); !r.Granted {
		t.Fatalf("URLs only overlap exactly: %+v", r)
	}
}

// Handing off work you only logged an intent for (no lease) still records the
// note for the next claimer; MCP clients did this before leases existed.
func TestCoordination_HandoffWithoutLeaseRecordsNote(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	email := fmt.Sprintf("handoff-nolease-%d@example.com", time.Now().UnixNano())
	if err := e.st.EnsureUser(ctx, email, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u, err := e.st.GetUserByEmail(ctx, email, uuid.Nil)
	if err != nil || u == nil {
		t.Fatalf("user: %v", err)
	}
	a, b := newAgent(t), newAgent(t)
	for _, ag := range []*agent{&a, &b} {
		if err := e.st.CreateIdentity(ctx, ag.id, &u.ID, uuid.Nil); err != nil {
			t.Fatal(err)
		}
	}
	code, body := e.do(&a, "v2", "POST", "/coordination/handoff", j(map[string]any{"resource": "docs", "summary": "quickstart drafted"}), nil)
	e.must(200, code, body, "handoff without a lease")
	code, body = e.do(&b, "v2", "POST", "/coordination/claim", j(map[string]any{"resource": "docs", "intent": "finish"}), nil)
	e.must(200, code, body, "claim after handoff note")
	var r claimResponse
	_ = json.Unmarshal(body, &r)
	if !r.Granted || r.HandoffSummary != "quickstart drafted" || r.HandoffFrom != a.id {
		t.Fatalf("claim after a lease-less handoff = %+v", r)
	}
}

func TestCoordination_PurgeKeepsActiveLeases(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	scope := "agent:purge-" + uuid.NewString()
	if _, err := e.st.ClaimLease(ctx, scope, "keep.go", "a", "s", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.ClaimLease(ctx, scope, "old.go", "a", "s", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := e.st.ReleaseLease(ctx, scope, "a", "s", nil, "old.go", "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Pool.Exec(ctx, `UPDATE coordination_leases SET released_at = NOW() - INTERVAL '31 days' WHERE scope_key = $1 AND resource = 'old.go'`, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.PurgeCoordination(ctx, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.st.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM coordination_leases WHERE scope_key = $1`, scope).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after purge %d rows (err %v), want only the active lease", n, err)
	}
}
