# Memro for Claude Code

Long-term memory for Claude Code, shared with your other AI tools (Cursor, Codex,
Copilot and anything else on the same Memro account).

```
/plugin install memro --marketplace memrohq/memro
```

On older Claude Code versions, add the marketplace first:
`/plugin marketplace add memrohq/memro`, then `/plugin install memro@memro`.

When you enable it, Claude Code asks for your **Memro API key** (`memro_sk_…`) and
keeps it in your system keychain. Create one in the dashboard (app.memro.co → API
keys), or run `npx memro-cli connect` and copy it from `~/.memro/credentials.json`.

## What it does

- **Adds the Memro MCP server** (`recall`, `remember`, task claims and more).
- **Recalls the project at the start of every session.** A startup hook searches
  Memro for this project's decisions, preferences and current work and gives
  them to Claude, along with a short note on when to recall and remember. It
  never blocks a session: with no key or no connection it adds just the note.

If you also ran `npx memro-cli connect` for Claude Code, its tools would appear
twice; the plugin tells you so. Keep one: `npx memro-cli disconnect --client claude-code`.

[Privacy](https://memro.co/privacy.html) · [Terms](https://memro.co/terms.html) ·
Source-available under PolyForm Noncommercial 1.0.0.
