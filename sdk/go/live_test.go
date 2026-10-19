package memro

import (
	"context"
	"os"
	"strings"
	"testing"
)

// End-to-end against a running server; set MEMRO_TEST_BASE_URL to run.
func TestLiveEndToEnd(t *testing.T) {
	base := os.Getenv("MEMRO_TEST_BASE_URL")
	if base == "" {
		t.Skip("MEMRO_TEST_BASE_URL not set")
	}
	ctx := context.Background()
	t.Setenv("MEMRO_API_KEY", "")
	opts := &ClientOptions{BaseURL: base}
	a, err := CreateAgent(NewClient(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DeleteAgent(ctx)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	m, err := a.Remember(ctx, "The billing service is written in Go", &RememberOptions{Type: MemoryTypeSemantic})
	must(err)
	_, err = a.Remember(ctx, "Ana prefers short answers", &RememberOptions{Type: MemoryTypeProfile})
	must(err)
	res, err := a.Recall(ctx, "billing service", &RecallOptions{Mode: SearchKeyword})
	must(err)
	if len(res) == 0 || res[0].MemoryID != m.ID {
		t.Fatalf("recall = %+v", res)
	}
	tr, err := a.RecallTemporal(ctx, "billing service", nil)
	must(err)
	if len(tr) == 0 {
		t.Fatal("no temporal results")
	}
	st, err := a.Stats(ctx)
	must(err)
	if st.TotalMemories != 2 {
		t.Fatalf("stats = %+v", st)
	}
	ex, err := a.Export(ctx)
	must(err)
	if ex.Total != 2 {
		t.Fatalf("export total = %d", ex.Total)
	}
	_, err = a.Reason(ctx, "what is the billing service written in")
	must(err)
	_, err = a.Timeline(ctx, m.ID)
	must(err)
	_, err = a.MemoryChain(ctx, m.ID)
	must(err)
	_, err = a.Evolve(ctx)
	must(err)
	_, err = a.Usage(ctx)
	must(err)
	_, err = a.AnalyzeConflicts(ctx)
	must(err)
	up, err := a.UploadFile(ctx, "notes.txt", "text/plain", []byte("Quarterly planning starts in March"), true)
	must(err)
	if up.ExtractedText == nil || !strings.Contains(*up.ExtractedText, "Quarterly") || up.MemoryID == "" {
		t.Fatalf("upload = %+v", up)
	}
	must(a.AddWebhook(ctx, "https://hooks.example.com/memro"))
	hooks, err := a.Webhooks(ctx)
	must(err)
	if len(hooks) != 1 {
		t.Fatalf("hooks = %v", hooks)
	}
	if ok, err := a.RemoveWebhook(ctx, "https://hooks.example.com/memro"); !ok || err != nil {
		t.Fatalf("remove webhook = %v %v", ok, err)
	}
	if ok, err := a.DeleteMemory(ctx, m.ID); !ok || err != nil {
		t.Fatalf("delete = %v %v", ok, err)
	}
	if ok, err := a.DeleteMemory(ctx, m.ID); ok || err != nil {
		t.Fatalf("second delete = %v %v", ok, err)
	}

	key, err := a.Client.CreateAPIKey(ctx, "go sdk test", a.Signer())
	must(err)
	keyed, err := AgentFromAPIKey(ctx, key, &ClientOptions{BaseURL: base})
	must(err)
	if keyed.AgentID != a.AgentID {
		t.Fatal("API key acts as a different agent")
	}
	_, err = keyed.Remember(ctx, "Deploys happen on Tuesdays", nil)
	must(err)

	other, err := CreateAgent(NewClient(&ClientOptions{BaseURL: base}))
	must(err)
	defer other.DeleteAgent(ctx)
	got, err := other.Recall(ctx, "Tuesdays deploys", &RecallOptions{Mode: SearchKeyword})
	must(err)
	if len(got) != 0 {
		t.Fatalf("another agent sees our memories: %+v", got)
	}
	if _, err := other.Client.ListMemories(ctx, a.AgentID, nil, other.Signer()); err == nil {
		t.Fatal("another agent listed our memories")
	}
}
