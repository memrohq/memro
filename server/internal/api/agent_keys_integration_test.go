package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// ADR 0013 step 1: an agent's identity is separate from the keys allowed to
// sign for it.

type key struct {
	pub  string
	priv ed25519.PrivateKey
}

func newKey(t *testing.T) key {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key{hex.EncodeToString(pub), priv}
}

// as signs a request for agentID with k, naming it in X-Key-Id.
func (e *env) as(agentID string, k key, method, path string, body []byte) (int, []byte) {
	e.t.Helper()
	return e.do(&agent{id: agentID, priv: k.priv}, "v2", method, path, body, map[string]string{"X-Key-Id": k.pub})
}

// addKeyBody is a POST /identity/{id}/keys body with a valid proof of
// possession by k (unless proofBy is given).
func addKeyBody(agentID string, k key, role string, proofBy ...key) []byte {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	signer := k
	if len(proofBy) > 0 {
		signer = proofBy[0]
	}
	sig := ed25519.Sign(signer.priv, []byte("memro-agent-key\n"+agentID+"\n"+k.pub+"\n"+ts))
	return j(map[string]string{"public_key": k.pub, "role": role, "proof": hex.EncodeToString(sig), "proof_timestamp": ts})
}

type keyRow struct {
	PublicKey string     `json:"public_key"`
	Role      string     `json:"role"`
	RevokedAt *time.Time `json:"revoked_at"`
}

func (e *env) keys(a agent) []keyRow {
	e.t.Helper()
	code, body := e.do(&a, "v2", "GET", "/identity/"+a.id+"/keys", nil, nil)
	e.must(200, code, body, "list keys")
	var out []keyRow
	_ = json.Unmarshal(body, &out)
	return out
}

func (e *env) auditActions(agentID string) map[string]int {
	e.t.Helper()
	rows, err := e.st.Pool.Query(context.Background(),
		`SELECT action, count(*) FROM audit_log WHERE agent_id = $1 AND action LIKE 'agent.key.%' GROUP BY action`, agentID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		_ = rows.Scan(&a, &n)
		out[a] = n
	}
	return out
}

// Existing agents and SDKs keep working: the agent ID is its first signing
// key, used when no X-Key-Id is sent. Agents that existed before migration
// 0027 get that key from its backfill.
func TestAgentKeysAreBackwardCompatible(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	ks := e.keys(a)
	if len(ks) != 1 || ks[0].PublicKey != a.id || ks[0].Role != "signing" || ks[0].RevokedAt != nil {
		t.Fatalf("keys of a new agent = %+v, want its ID as the one signing key", ks)
	}
	e.remember(a, "Signed the old way, without X-Key-Id", "")

	// An agent created before the migration (no trigger), then backfilled.
	ctx := context.Background()
	old := newAgent(t)
	for _, sql := range []string{
		`ALTER TABLE agent_identities DISABLE TRIGGER agent_identities_original_key`,
		`INSERT INTO agent_identities (id, tenant_id) VALUES ('` + old.id + `', '00000000-0000-0000-0000-000000000000')`,
		`ALTER TABLE agent_identities ENABLE TRIGGER agent_identities_original_key`,
	} {
		if _, err := e.st.Pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	code, body := e.do(&old, "v2", "GET", "/auth/whoami", nil, nil)
	e.must(401, code, body, "an agent with no key rows is not trusted")
	if _, err := e.st.Pool.Exec(ctx, `INSERT INTO agent_keys (public_key, agent_id, role)
		SELECT id, id, 'signing' FROM agent_identities ON CONFLICT (public_key) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	code, body = e.do(&old, "v2", "GET", "/auth/whoami", nil, nil)
	e.must(200, code, body, "backfilled agent signs as before")
}

// Rotation keeps the agent's ID and memories; revoking the original key also
// ends the no-X-Key-Id path.
func TestAgentKeyRotation(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	e.remember(a, "Memro stores memories in Postgres", "semantic")

	k2 := newKey(t)
	code, body := e.do(&a, "v2", "POST", "/identity/"+a.id+"/keys", addKeyBody(a.id, k2, "signing"), nil)
	e.must(201, code, body, "add a signing key")
	code, body = e.as(a.id, k2, "GET", "/memory/agent/"+a.id, nil)
	e.must(200, code, body, "new key reads the same memories")
	var mems []map[string]any
	_ = json.Unmarshal(body, &mems)
	if len(mems) != 1 {
		t.Fatalf("memories via the new key = %d, want 1", len(mems))
	}

	code, body = e.as(a.id, k2, "DELETE", "/identity/"+a.id+"/keys/"+a.id, nil)
	e.must(204, code, body, "revoke the original key with the new one")
	code, body = e.do(&a, "v2", "GET", "/auth/whoami", nil, nil)
	e.must(401, code, body, "original key, no X-Key-Id, after revocation")
	code, body = e.as(a.id, key{a.id, a.priv}, "GET", "/auth/whoami", nil)
	e.must(401, code, body, "original key named in X-Key-Id, after revocation")
	code, body = e.as(a.id, k2, "GET", "/auth/whoami", nil)
	e.must(200, code, body, "rotated key")
	var who struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal(body, &who)
	if who.AgentID != a.id {
		t.Fatalf("agent ID changed on rotation: %s", who.AgentID)
	}

	// Keys are never reused.
	code, body = e.as(a.id, k2, "POST", "/identity/"+a.id+"/keys", addKeyBody(a.id, key{a.id, a.priv}, "signing"))
	e.must(409, code, body, "re-adding a revoked key")

	if got := e.auditActions(a.id); got["agent.key.added"] != 1 || got["agent.key.revoked"] != 1 {
		t.Fatalf("audit = %v", got)
	}
}

// Nobody can attach a key whose private key they do not hold, a key belongs to
// one agent, and stale or misdirected proofs fail.
func TestAgentKeyProofOfPossession(t *testing.T) {
	e := setup(t)
	a, b := newAgent(t), newAgent(t)
	e.register(a)
	e.register(b)
	k := newKey(t)
	path := "/identity/" + a.id + "/keys"

	code, body := e.do(&a, "v2", "POST", path, j(map[string]string{"public_key": k.pub}), nil)
	e.must(400, code, body, "no proof")
	code, body = e.do(&a, "v2", "POST", path, addKeyBody(a.id, k, "signing", newKey(t)), nil)
	e.must(400, code, body, "proof signed by another key")
	code, body = e.do(&a, "v2", "POST", path, addKeyBody(b.id, k, "signing"), nil)
	e.must(400, code, body, "proof made for another agent")
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	sig := ed25519.Sign(k.priv, []byte("memro-agent-key\n"+a.id+"\n"+k.pub+"\n"+stale))
	code, body = e.do(&a, "v2", "POST", path, j(map[string]string{"public_key": k.pub, "proof": hex.EncodeToString(sig), "proof_timestamp": stale}), nil)
	e.must(400, code, body, "stale proof")
	code, body = e.do(&a, "v2", "POST", path, j(map[string]string{"public_key": k.pub, "role": "admin"}), nil)
	e.must(400, code, body, "unknown role")

	// b's key belongs to b, even with a valid proof by its holder.
	code, body = e.do(&a, "v2", "POST", path, addKeyBody(a.id, key{b.id, b.priv}, "signing"), nil)
	e.must(409, code, body, "another agent's key")
	// A's added key cannot become a separate agent.
	code, body = e.do(&a, "v2", "POST", path, addKeyBody(a.id, k, "signing"), nil)
	e.must(201, code, body, "valid proof")
	code, body = e.do(nil, "", "POST", "/identity", j(map[string]string{"public_key": k.pub}), nil)
	e.must(409, code, body, "registering another agent's key as a new agent")
}

// A recovery key manages keys and nothing else, and can restore an agent that
// lost every signing key.
func TestRecoveryKey(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	e.remember(a, "Recovery keys only manage keys", "semantic")
	r := newKey(t)
	code, body := e.do(&a, "v2", "POST", "/identity/"+a.id+"/keys", addKeyBody(a.id, r, "recovery"), nil)
	e.must(201, code, body, "add a recovery key")

	for _, c := range []struct{ method, path string }{
		{"POST", "/memory"}, {"GET", "/search?query=keys"}, {"GET", "/memory/agent/" + a.id},
		{"GET", "/export/" + a.id}, {"GET", "/auth/whoami"}, {"POST", "/api-keys"}, {"DELETE", "/identity/" + a.id},
	} {
		var b []byte
		if c.method == "POST" {
			b = j(map[string]string{"content": "x", "name": "x"})
		}
		code, body = e.as(a.id, r, c.method, c.path, b)
		e.must(403, code, body, "recovery key: "+c.method+" "+c.path)
	}
	code, body = e.as(a.id, r, "GET", "/identity/"+a.id+"/keys", nil)
	e.must(200, code, body, "recovery key lists keys")

	// The signing key is lost: it can be revoked because a recovery key remains,
	// and the recovery key adds a new one.
	code, body = e.do(&a, "v2", "DELETE", "/identity/"+a.id+"/keys/"+a.id, nil, nil)
	e.must(204, code, body, "revoke the only signing key while a recovery key exists")
	k3 := newKey(t)
	code, body = e.as(a.id, r, "POST", "/identity/"+a.id+"/keys", addKeyBody(a.id, k3, "signing"))
	e.must(201, code, body, "recovery key restores a signing key")
	code, body = e.as(a.id, k3, "GET", "/memory/agent/"+a.id, nil)
	e.must(200, code, body, "memories are intact after recovery")
	if got := e.auditActions(a.id); got["agent.key.recovered"] != 1 {
		t.Fatalf("audit = %v, want agent.key.recovered", got)
	}
}

// Nobody locks an agent out by accident: the last signing key of an unowned
// agent stays while there is no recovery key; an owned agent's owner can
// always add a key.
func TestLastSigningKeyIsProtected(t *testing.T) {
	e := setup(t)
	a := newAgent(t)
	e.register(a)
	code, body := e.do(&a, "v2", "DELETE", "/identity/"+a.id+"/keys/"+a.id, nil, nil)
	e.must(409, code, body, "last signing key of an unowned agent")
	code, body = e.do(&a, "v2", "DELETE", "/identity/"+a.id+"/keys/"+newKey(t).pub, nil, nil)
	e.must(404, code, body, "unknown key")

	owned, session := e.dashboardSession(uniqueEmail("keys-owner"))
	code, body = e.do(nil, "", "DELETE", "/identity/"+owned.id+"/keys/"+owned.id, nil, bearer(session))
	e.must(204, code, body, "owner revokes the only key (the owner can add another)")
	k := newKey(t)
	code, body = e.do(nil, "", "POST", "/identity/"+owned.id+"/keys", addKeyBody(owned.id, k, "signing"), bearer(session))
	e.must(201, code, body, "owner adds a key from the dashboard")
	code, body = e.as(owned.id, k, "GET", "/auth/whoami", nil)
	e.must(200, code, body, "owner-added key signs")
}

// API keys never manage keys; other agents and other people cannot either.
func TestOnlyTheAgentOrItsOwnerManageKeys(t *testing.T) {
	e := setup(t)
	owned, session := e.dashboardSession(uniqueEmail("keys-a"))
	apiKey := e.createKey(session)
	_, otherSession := e.dashboardSession(uniqueEmail("keys-b"))
	stranger := newAgent(t)
	e.register(stranger)
	k := newKey(t)
	path := "/identity/" + owned.id + "/keys"

	for name, call := range map[string]func(method, p string, b []byte) (int, []byte){
		"api key":      func(m, p string, b []byte) (int, []byte) { return e.do(nil, "", m, p, b, bearer(apiKey)) },
		"other person": func(m, p string, b []byte) (int, []byte) { return e.do(nil, "", m, p, b, bearer(otherSession)) },
		"other agent":  func(m, p string, b []byte) (int, []byte) { return e.do(&stranger, "v2", m, p, b, nil) },
	} {
		code, body := call("GET", path, nil)
		e.must(403, code, body, name+": list keys")
		code, body = call("POST", path, addKeyBody(owned.id, k, "signing"))
		e.must(403, code, body, name+": add key")
		code, body = call("DELETE", path+"/"+owned.id, nil)
		e.must(403, code, body, name+": revoke key")
	}
}
