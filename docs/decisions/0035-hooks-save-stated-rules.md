# 0035. Hooks save the rules a user states, and show them every prompt
Date: 2026-11-18 · Status: Accepted; where rules are stored superseded by ADR 0037

## Context
Memro's memory reached the next agent only when the model chose to call
`remember`, and then only when the next agent searched with words the memory
shared. The relay benchmark (journal 2026-11-17, §7) measured the cost: with
300 earlier facts in memory, a plain `NOTES.md` read whole kept a fresh agent
from a known mistake 10/10, Memro's tools 6/10. Of Memro's 4 misses, 2 were an
agent that never saved the user's rule and 2 were a search for the task
("formatMoney … money formatting") that missed a rule about `Intl`.

The coordination hooks (ADR 0019 §7) already run on every prompt and every edit
in Claude Code, Codex, Cursor, Gemini CLI and OpenCode, so they can do both
steps without the model.

## Decision
The hooks (`cli/src/hook.js`) gain three steps:
1. **Save stated rules.** On each prompt, sentences containing "never",
   "always", "from now on" or "as a rule" are saved with the sentence after
   them (usually the reason), as procedural memories
   `Rule for <repo>: <the user's words>` with metadata
   `{source: "memro-hook", repo}`. The user's words are kept as written: in the
   benchmark an agent paraphrasing a rule narrowed it ("cart code must avoid
   Intl") and the next agent broke it elsewhere.
2. **Show the repo's rules at every prompt**, as context before the agent
   starts, so they reach it whatever it would have searched for. `<repo>` is
   the origin's name, so every clone, worktree and tool of a repo shares them.
3. **Show a file's memories on its first edit** in a session: a search for
   `<repo>/<path>`, keeping results with similarity ≥ 0.4 (ADR 0033), rules
   excluded since step 2 shows them.

No server change. Like the rest of the hook, every step fails open.

## Consequences
- Sentences from users' prompts are stored in their Memro account. This is
  stated in the CHANGELOG and the CLI README; `memro connect --no-hooks` turns
  the hooks off. Whether to add a separate opt-out for rule saving is open
  (PR #98).
- Detection is keyword-based: a rule phrased without those words isn't saved,
  and an instruction with them ("always run the tests") is saved as a rule.
- Listing a repo's rules was a keyword search capped at 20. In the 3,000-fact
  smoke test (2026-11-19) a repo with 102 rules got 20 others and never the
  trap's, so the server gained `GET /rules?repo=…`: the hook-saved rules for
  that repo (metadata `source`/`repo`), newest first, up to 200 (about 5,000
  tokens per prompt). The hook falls back to the keyword search on a server
  without it.
- Measured (journal §8, 45 relays): with 300 facts in memory, Memro with the
  hooks 10/10, a notes file 9/10, Memro's tools alone 8/10.
