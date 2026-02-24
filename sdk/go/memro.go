// Package memro provides the official Go SDK for the memro protocol —
// AI agent memory infrastructure.
//
// Quickstart:
//
//agent, err := memro.CreateAgent(nil)
//if err != nil { log.Fatal(err) }
//
//mem, err := agent.Remember(ctx, "User prefers Go", nil)
//results, err := agent.Recall(ctx, "user preferences", nil)
//agent.Save("identity.json")
package memro
