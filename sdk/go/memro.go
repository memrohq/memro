// Package memro is the official Go SDK for Memro, long-term memory for AI agents.
//
// With an API key from `npx memro connect` or the dashboard:
//
//	agent, err := memro.AgentFromAPIKey(ctx, os.Getenv("MEMRO_API_KEY"), nil)
//	mem, err := agent.Remember(ctx, "User prefers Go", &memro.RememberOptions{Type: memro.MemoryTypeProfile})
//	results, err := agent.Recall(ctx, "language preferences", nil)
//
// Or with a self-owned identity whose private key never leaves this machine:
//
//	agent, err := memro.CreateAgent(nil)
//	err = agent.Save("identity.json")
//
// See https://github.com/memrohq/memro/tree/main/docs for the full guide.
package memro
