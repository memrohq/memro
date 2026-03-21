# Memro: Opinionated Developer Guide 🧭

Memro gives your AI agents **memory that evolves and reasons over time**. This guide shows you exactly how to use it for the most common use cases.

## 🧭 The Golden Rule: reason() vs recall()

| Tool | When to use | Example |
| :--- | :--- | :--- |
| **recall()** | Simple lookup of facts or past events. | "What is the user's name?" |
| **reason()** | Understanding patterns, causality, or "why". | "Why did the user change their preference?" |

---

## 🤖 1. Build a Memory-Enabled Chatbot
Give your chatbot a "continuous stream of consciousness."

### The Pattern:
1. Every user message: `remember(content="User said: ...", type="episodic")`
2. Every bot thought: `recall(query="Does the user have any existing preferences for [topic]?")`
3. Every 24 hours: `evolve()` to turn chat logs into semantic facts.

```python
# Quick Start
memro.remember("I only work in the mornings", type="auto") # Auto-classifies as PROFILE
```

## 🧠 2. Track User Preferences Over Time
Stop asking the user the same questions. Let Memro observe and distill.

### The Pattern:
1. Don't manualy tag. Use `type="auto"`.
2. Memro will identify "I like dark mode" as a `profile` memory.
3. Use `reason(query="What are the user's UI preferences?")` to get a synthesized answer with a causal trace.

## 🤝 3. Multi-Agent Coordination
Let Agent A leave "causal breadcrumbs" for Agent B.

### The Pattern:
1. Agent A (Researcher): `remember("Identified a bottleneck in the API", type="episodic")`
2. Agent B (Fixer): `reason("Why is the system slow?")`
3. Memro connects the breadcrumbs through the causal graph and explains the link in the `why` field.

---

## ⚠️ Soft Caps & Safety
- **Soft Cap**: 1,000 memories per agent.
- **Why?** To prevent graph bloat and ensure high-quality reasoning.
- **Check usage**: Use the `/usage` endpoint to monitor your graph health.

> [!TIP]
> **Positioning**: Memro is not a vector database. It is a **cognitive runtime**. Use it when your agent needs to *learn* and *adapt*, not just *store*.
