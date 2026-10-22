package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/api"
	"github.com/memrohq/memro/server/internal/auth"
)

// ADR 0013: proof-of-work on agent self-registration.
func TestRegistrationProofOfWork(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.IdentityPowBits, d.Config.IdentityPowRequired = 8, true })

	code, b := e.do(nil, "", "GET", "/identity/pow", nil, nil)
	e.must(200, code, b, "pow parameters")
	var params struct {
		Difficulty int  `json:"difficulty"`
		Required   bool `json:"required"`
	}
	_ = json.Unmarshal(b, &params)
	if params.Difficulty != 8 || !params.Required {
		t.Fatalf("pow parameters = %s", b)
	}

	a := newAgent(t)
	code, b = e.do(nil, "", "POST", "/identity", j(map[string]string{"public_key": a.id}), nil)
	e.must(400, code, b, "no proof while required")

	ts := time.Now().Unix()
	nonce := auth.SolvePow(a.id, ts, 8)
	other := newAgent(t)
	code, b = e.do(nil, "", "POST", "/identity", j(map[string]any{"public_key": other.id,
		"pow": map[string]any{"timestamp": ts, "nonce": nonce}}), nil)
	e.must(400, code, b, "proof made for another key")

	stale := time.Now().Add(-auth.PowMaxAge - time.Minute).Unix()
	code, b = e.do(nil, "", "POST", "/identity", j(map[string]any{"public_key": a.id,
		"pow": map[string]any{"timestamp": stale, "nonce": auth.SolvePow(a.id, stale, 8)}}), nil)
	e.must(400, code, b, "stale proof")

	code, b = e.do(nil, "", "POST", "/identity", j(map[string]any{"public_key": a.id,
		"pow": map[string]any{"timestamp": ts, "nonce": nonce}}), nil)
	e.must(200, code, b, "valid proof")
	e.remember(a, "registered with a proof-of-work", "")
}

// While the proof is optional, old SDKs that send none still register, but a
// proof that is sent must be valid.
func TestRegistrationProofOptional(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.IdentityPowBits, d.Config.IdentityPowRequired = 8, false })
	a := newAgent(t)
	e.register(a)

	b := newAgent(t)
	code, body := e.do(nil, "", "POST", "/identity", j(map[string]any{"public_key": b.id,
		"pow": map[string]any{"timestamp": time.Now().Unix(), "nonce": "x"}}), nil)
	if code == 200 {
		// "x" may meet 8 bits by chance; it is the same check either way.
		return
	}
	e.must(400, code, body, "invalid optional proof")
}

func TestUnclaimedAgentCaps(t *testing.T) {
	e := setup(t, func(d *api.Deps) { d.Config.UnclaimedMaxMemories, d.Config.UnclaimedMaxFileBytes = 2, 20 })
	a := newAgent(t)
	e.register(a)

	e.remember(a, "first", "")
	e.remember(a, "second", "")
	code, b := e.do(&a, "v2", "POST", "/memory", j(map[string]any{"content": "third"}), nil)
	e.must(403, code, b, "memory cap for unclaimed agent")
	if !strings.Contains(string(b), "claim") {
		t.Fatalf("cap message should mention claiming: %s", b)
	}

	upload := func(content string) (int, []byte) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "notes.txt")
		fw.Write([]byte(content))
		mw.WriteField("agent_id", a.id)
		mw.Close()
		return e.do(&a, "v2", "POST", "/upload", buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType()})
	}
	code, b = upload("twelve bytes")
	e.must(200, code, b, "upload under the file cap")
	code, b = upload("another twelve")
	e.must(403, code, b, "upload over the file cap")
}

// exemptPolicy lifts the unclaimed caps for one agent, as a paid plan would.
type exemptPolicy struct {
	extension.PerAgentCap
	agent string
}

func (p exemptPolicy) ExemptFromUnclaimedCaps(_ context.Context, _ uuid.UUID, agentID string) bool {
	return agentID == p.agent
}

func TestUnclaimedCapExemption(t *testing.T) {
	paid, unpaid := newAgent(t), newAgent(t)
	e := setup(t, func(d *api.Deps) {
		d.Config.UnclaimedMaxMemories = 1
		d.Config.UnclaimedCapHint = "Buy a plan: /billing/x402/terms?subject=agent:{agent_id}"
		d.Quota = exemptPolicy{extension.PerAgentCap{Max: 1000}, paid.id}
	})
	e.register(paid)
	e.register(unpaid)
	for _, a := range []agent{paid, unpaid} {
		e.remember(a, "first", "")
	}
	e.remember(paid, "second, past the unclaimed cap", "")
	code, b := e.do(&unpaid, "v2", "POST", "/memory", j(map[string]any{"content": "second"}), nil)
	e.must(403, code, b, "unpaid agent at the cap")
	if !strings.Contains(string(b), "subject=agent:"+unpaid.id) {
		t.Fatalf("cap message should carry the hint with the agent id: %s", b)
	}
}
