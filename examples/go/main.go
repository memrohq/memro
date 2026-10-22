package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/memrohq/memro/sdk/go"
)

func main() {
	fmt.Println("=== Memro Go SDK: Register -> Remember -> Recall ===")

	baseURL := os.Getenv("MEMRO_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.memro.co"
	}
	apiKey := os.Getenv("MEMRO_API_KEY")

	ctx := context.Background()
	var agent *memro.Agent
	var err error

	client := memro.NewClient(&memro.ClientOptions{BaseURL: baseURL})

	if apiKey != "" {
		fmt.Printf("Connecting to Memro (%s) using MEMRO_API_KEY...\n", baseURL)
		agent, err = memro.AgentFromAPIKey(ctx, apiKey, &memro.ClientOptions{BaseURL: baseURL})
	} else if _, statErr := os.Stat("identity.json"); statErr == nil {
		fmt.Printf("Connecting to Memro (%s) using saved identity.json...\n", baseURL)
		agent, err = memro.AgentFromFile("identity.json", client)
	} else if os.Getenv("MEMRO_AGENT_ID") != "" && os.Getenv("MEMRO_PRIVATE_KEY") != "" {
		fmt.Printf("Connecting to Memro (%s) using agent environment credentials...\n", baseURL)
		agent, err = memro.AgentFromEnv(client)
	} else {
		fmt.Printf("Registering autonomous sovereign agent on %s (proof-of-work)...\n", baseURL)
		agent, err = memro.CreateAgent(client)
		if err == nil {
			_ = agent.Save("identity.json")
		}
	}

	if err != nil {
		log.Fatalf("Failed to initialize agent: %v", err)
	}

	agentID := agent.AgentID
	displayID := agentID
	if len(agentID) > 24 {
		displayID = agentID[:16] + "..." + agentID[len(agentID)-8:]
	}
	fmt.Printf("Agent ID (Ed25519 Public Key): %s\n", displayID)

	// 2. Remember a typed procedural observation
	observation := "Always run database migrations before deploying billing services."
	fmt.Printf("\nStoring observation: %q [type: procedural]\n", observation)
	_, err = agent.Remember(ctx, observation, &memro.RememberOptions{
		Type: memro.MemoryTypeProcedural,
	})
	if err != nil {
		log.Fatalf("Failed to remember: %v", err)
	}
	fmt.Println("Memory stored successfully.")

	// 3. Recall verified context before prompting
	query := "deploying billing service"
	fmt.Printf("\nRecalling context for query: %q...\n", query)
	results, err := agent.Recall(ctx, query, &memro.RecallOptions{
		Limit: 5,
	})
	if err != nil {
		log.Fatalf("Failed to recall: %v", err)
	}

	fmt.Printf("Retrieved %d relevant memories:\n", len(results))
	for idx, r := range results {
		fmt.Printf("  %d. %s (score: %.3f)\n", idx+1, r.Content, r.Score)
	}

	fmt.Println("\n✓ Run completed successfully in < 5 seconds.")
}
