# Quickstart

## Give your coding tools a memory (1 minute)

> **Before the packages are published** (`memro` and `@memro/sdk` on npm,
> `memro` on PyPI), use the source instead:
> ```bash
> git clone https://github.com/memrohq/memro && node memro/cli/bin/memro.js connect
> pip install "git+https://github.com/memrohq/memro#subdirectory=sdk/python"
> (cd memro/sdk/js && npm install && npm run build) && npm install ./memro/sdk/js
> ```
> The Go SDK installs from GitHub as shown below.

```bash
npx memro connect
```

This opens the dashboard so you can sign in with your email, asks you to
confirm the code shown in your terminal, then adds Memro to every supported
tool it finds on your machine: Claude Code, Codex, Cursor, Antigravity,
OpenCode, Gemini CLI, VS Code, Windsurf and Claude Desktop. Restart the tool
(or run `/mcp` in Claude Code) and ask it to remember something:

> Remember that our API uses snake_case JSON and that deploys go out on Tuesdays.

In a later session, ask *"How do we name JSON fields?"*. The tool calls
Memro's `recall` and answers from memory.

| Command | |
|---|---|
| `npx memro connect --client cursor,claude-code` | Only these tools |
| `npx memro status` | Show the account and which tools are configured |
| `npx memro disconnect` | Remove Memro from your tools and delete the local key |
| `npx memro connect --api http://localhost:8081` | Use a self-hosted server |

Setting a tool up by hand instead? See [MCP server](mcp.md).

## Use Memro from code (5 minutes)

### 1. Get an API key

Run `npx memro connect` (the key is saved in `~/.memro/credentials.json`), or
open [app.memro.co](https://app.memro.co) → **API keys** → **New key**.
Keys look like `memro_sk_…` and are shown once.

```bash
export MEMRO_API_KEY=memro_sk_...
```

### 2. Install an SDK

```bash
pip install memro          # Python 3.8+
npm install @memro/sdk     # Node.js 18+, Deno, Bun, browsers
go get github.com/memrohq/memro/sdk/go
```

### 3. Remember and recall

**Python**

```python
from memro import Agent

agent = Agent.from_env()  # reads MEMRO_API_KEY

agent.remember("Ana leads the platform team and prefers short answers", type="profile")
agent.remember("The billing service was rewritten in Go in March 2026", type="semantic")

for r in agent.recall("who leads platform?"):
    print(f"{r.score:.2f}  {r.content}")
```

**TypeScript**

```ts
import { Agent } from '@memro/sdk';

const agent = await Agent.fromEnv();
await agent.remember('Ana leads the platform team and prefers short answers', { type: 'profile' });
const results = await agent.recall('who leads platform?');
```

**Go**

```go
agent, err := memro.AgentFromAPIKey(ctx, os.Getenv("MEMRO_API_KEY"), nil)
_, err = agent.Remember(ctx, "Ana leads the platform team", &memro.RememberOptions{Type: memro.MemoryTypeProfile})
results, err := agent.Recall(ctx, "who leads platform?", nil)
```

**curl**

```bash
curl -s https://api.memro.co/memory \
  -H "Authorization: Bearer $MEMRO_API_KEY" -H "Content-Type: application/json" \
  -d '{"content": "Ana leads the platform team", "memory_type": "profile"}'

curl -s "https://api.memro.co/search?query=who+leads+platform" \
  -H "Authorization: Bearer $MEMRO_API_KEY"
```

### 4. Put memory in the prompt

The usual loop: recall before calling the model, remember what was learned after.

```python
context = "\n".join(f"- {r.content}" for r in agent.recall(user_message, limit=5))
reply = llm(system=f"Relevant memory:\n{context}", user=user_message)
agent.remember(f"User asked: {user_message}", type="episodic")
```

Next: [Concepts](concepts.md) · [API reference](api-reference.md) · [MCP server](mcp.md)
