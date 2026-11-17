# Relay benchmark (design, pilot)

Does an agent that picks up someone else's work avoid their known mistakes?
The collision benchmark tests locking, which Claude Code mostly handles by
itself (2026-11-17 journal). This one tests what only memory can do: carry
what was learned in one session into the next one, in another tool or on
another machine.

## The relay

Each run is two agents in sequence on the shop fixture
(`bench/coordination/fixture`):

1. **Agent A** gets a task and, in its prompt, a fact from the user that the
   code doesn't show: a decision, a constraint, or an approach that already
   failed. It does its task and ends.
2. **Agent B** is a fresh session with no chat history. Its task is in the same
   area, and the obvious way to do it breaks A's fact.
3. A hidden acceptance test checks B's work, including whether it respected
   the fact.

## Traps

A's own task never touches what the fact is about, so the fact can only reach
B through memory. In pilot v1 (2026-11-17) A's task was in the same code, A
wrote the fact into comments ("no Intl: POS builds lack ICU"), and B copied
them: every arm, `none` included, avoided every trap.

| Trap | A's task | A is told, in passing | B's task | Obvious mistake | Hidden test checks |
|---|---|---|---|---|---|
| R1 project rule | `itemCount` | "Never use `Intl` or `toLocaleString` anywhere: the POS devices run Node without ICU." | `formatMoney(cents, locale, currency)` for en-US/USD and de-DE/EUR | `Intl.NumberFormat` | Output right and no `Intl` in `src/` |
| R2 decision before code | `removeItem` | "When we add coupons or any discount, the total rounds down to a whole cent." | `couponTotal(cart, percent)` | `Math.round` | Totals ending in .5 or more round down |
| R3 migration elsewhere | `hasItem` | "The mobile app already sends `unitPrice` instead of `price`; anything reading a price must accept both." | `itemSubtotal(item)` | Reads only `item.price` | Works with `unitPrice` |
| R4 control | `itemCount` | Nothing | `hasItem` | none | Task only |

Each trap's tests were checked against a correct and a naive solution: the
correct one passes all, the naive one does its task but fails the trap test.

## Arms

| Arm | How A's knowledge can reach B |
|---|---|
| none | It can't: B starts fresh. The floor. |
| notes | A and B are told to keep what a future agent needs in `NOTES.md` and to read it first, as with a `CLAUDE.md` or `AGENTS.md`. The strong, free baseline Memro must beat. |
| memro | A and B have Memro's MCP tools with the standard project instructions (recall at the start, remember decisions). |

The recording instruction is word-for-word the same in `notes` and `memro`;
only the place differs. B works in the same checkout as A in the pilot.

**Stage 2**, run only after the pilot: B in a fresh clone of `main` (a
teammate on another machine, so A's uncommitted `NOTES.md` isn't there), and
B as Codex instead of Claude.

## Measures

Per run:
- **Trap avoided**: B's hidden test passes. The headline number.
- **Task done**: B's ordinary acceptance test passes.
- **A recorded it**: the fact's keyword is in `NOTES.md` or in the run's
  Memro memories. This separates "never written down" from "written but not
  read".
- **B looked**: B read `NOTES.md` or called `recall`.
- B's time and cost.

## Isolation

The `memro` arm runs against the local bench server, like LongMemEval: a fresh
Memro agent per run, deleted afterwards, so bench memories never reach the
user's real memory or the next run. Agents run with `--strict-mcp-config` and
only that MCP server, as in the collision benchmark. To check when building:
that the local server's MCP endpoint accepts a bench agent's key.

## Pilot size and cost

4 traps × 3 arms × 5 runs = 60 relays, 120 Claude Haiku sessions, about
$1–2 and 30–40 minutes. Arms are interleaved run by run.

## What would count as a result

- **Memro clearly beats notes** on R1–R3 (for example 90% vs 60% traps
  avoided): memory is the headline.
- **Memro and notes tie, both well above none**: in one checkout a notes file
  is enough. Memro's case then rests on stage 2, across clones and tools,
  which a notes file can't cross unless it's committed and merged.
- **No arm beats none**: the traps are too easy, or agents don't use memory
  without being made to. Fix the traps, or test hook-delivered memory (shown
  when a file is first touched) instead of relying on the agent to recall.
