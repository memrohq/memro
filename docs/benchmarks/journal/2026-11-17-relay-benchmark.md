# 2026-11-17: Relay benchmark — does memory stop a fresh agent repeating mistakes?

- **Date**: 2026-11-17
- **Harness**: `bench/relay/run.py` (design in `bench/relay/README.md`), branch `bench/relay`
- **Agents**: Claude Code 2.1.311, `--model haiku`, one at a time
- **Run directories** (local, not committed): `bench/runs/relay/pilot-20261117` (v1, stopped), `bench/runs/relay/pilot2-20261117` (v2 pilot, 5 relays), `bench/runs/relay/v2-20261118` (v2, 60 relays)
- **Cost**: v1 $0.19 for 23 relays; v2 $0.52 for 60 relays

## 1. Goal & hypothesis

The collision benchmark can't show what's unique about Memro: Claude Code's
own edit tool already refuses stale edits, so a shared folder ties Memro on
tasks landed (2026-11-17 coordination journal). Memro's distinct claim is
memory that outlives a session. Hypothesis: when one agent learns a fact that
the code doesn't show, a fresh agent with Memro respects it far more often
than one without, and at least as often as one with a notes file.

## 2. Setup

Each relay is two agents in sequence on the shop fixture:

1. **Agent A** gets a task and, in its prompt, a fact from the user.
2. **Agent B** is a fresh session. Its task's obvious solution breaks A's fact.
3. Hidden tests check A's task, B's task, and whether B respected the fact.

| Arm | How A's fact can reach B |
|---|---|
| none | It can't. The floor. |
| notes | A and B are told to keep and read `NOTES.md`, like a `CLAUDE.md`. The free baseline. |
| memro | A and B have Memro's `recall` and `remember` tools. |

The recording instruction is word for word the same in `notes` and `memro`;
only the place differs. Per relay the harness also records whether A wrote the
fact down and whether B looked it up.

**Isolation, two things found while building:**

- Claude Code's built-in auto-memory ("You have a persistent file-based
  memory…") is kept per project folder, so it would carry facts from A to B in
  every arm, `none` included. It is switched off in every relay repo with
  `{"autoMemoryEnabled": false}` in `.claude/settings.json`; a check prompt
  confirmed agents no longer see it.
- The memro arm uses one throwaway Memro agent on the live API
  (`Agent.create`, reached through a local stdio `memro-mcp`), not tied to any
  account. It is wiped before every relay and deleted at the end, also when a
  run is stopped (SIGTERM handler).

## 3. Pilot v1: traps too easy

A's task was in the same code as the fact (for example "add thousands
separators to `formatPrice`; don't use `Intl`"). After 22 of 60 relays:

| Arm | Traps avoided (R1–R3) | A wrote the fact down | B looked it up |
|---|---|---|---|
| none | 6/6 | – | – |
| notes | 6/6 | 4/6 | 6/6 |
| memro | 6/6 | 5/6 | 6/6 |

Every arm avoided every trap because A wrote the fact into the code it changed
(`// Avoids Intl/toLocaleString: POS Node builds ship without ICU`,
`Math.floor` beside "rounded down to a whole cent") and B copied the pattern.
The traps tested code reading, not memory. The run was stopped there.

Also found: the first R1 test flagged agents that only mentioned `Intl` in a
comment ("no Intl here"). It now strips comments and matches real use
(`Intl.`, `.toLocaleString(`).

## 4. Pilot v2: facts that leave no trace in code

A's own task never touches what the fact is about; the fact is mentioned in
passing, the way it happens in real work.

| Trap | A's task | A is told in passing | B's task | Obvious mistake |
|---|---|---|---|---|
| R1 project rule | `itemCount` | Never use `Intl` or `toLocaleString`: POS devices have no ICU | `formatMoney` for en-US/USD and de-DE/EUR | `Intl.NumberFormat` |
| R2 decision before code | `removeItem` | Discounted totals round down to a whole cent | `couponTotal(cart, percent)` | `Math.round` |
| R3 migration elsewhere | `hasItem` | The mobile app sends `unitPrice`, not `price`; accept both | `itemSubtotal(item)` | Reads only `item.price` |
| R4 control | `itemCount` | Nothing | `hasItem` | none |

Each trap's tests were checked against a correct and a naive solution: the
correct one passes everything, the naive one does its task but fails the trap
test. 5 runs × 4 traps × 3 arms = 60 relays, interleaved.

**Results** (2026-11-18, 60 relays):

| Trap | none | notes | memro |
|---|---|---|---|
| R1 project rule (no `Intl`) | 0/5 | 5/5 | 5/5 |
| R2 decision before code (round down) | 0/5 | 5/5 | 5/5 |
| R3 migration elsewhere (`unitPrice`) | 5/5 | 5/5 | 5/5 |
| R4 control | 5/5 | 5/5 | 5/5 |
| **R1–R3** | **5/15** | **15/15** | **15/15** |

A wrote the fact down in 15/15 notes relays and 14/15 memro relays; B looked
it up in every notes and memro relay. B did its task in all 60. Each arm cost
$0.13–0.14 for R1–R3.

- **Memory is the whole difference.** Without it, B broke the fact in all 10
  R1 and R2 relays: it used `Intl.NumberFormat` and `Math.round`, the obvious
  solutions. With either kind of memory it never did.
- **Memro ties a notes file** here, as the "tie" outcome in §5 predicted: in
  one checkout, on one machine, a file both agents read is enough.
- **R3 doesn't test memory.** B accepted `unitPrice` 5/5 even without it
  (the one memro relay where A saved nothing also passed), so the trap is too
  easy and is dropped from the comparison.
- The first-pilot worry (A not calling `remember`) didn't hold up: 14/15.

## 5. What would count as a result

- **memro clearly beats notes** on R1–R3: memory is the headline.
- **memro and notes tie, both well above none**: in one checkout a notes file
  is enough. Next is stage 2, with B in a fresh clone or as Codex, where an
  uncommitted notes file can't follow.
- **none still avoids the traps**: agents guess right without memory; the
  traps need to be harder, and B's work will show how.

**Next (stage 2)**: put Memro where a notes file in the repo can't follow,
each with the strongest fair file baseline:
- **Another person's agent**: B is a teammate's agent in a fresh clone; the
  notes arm commits `NOTES.md` with A's work, the memro arm shares a team space.
- **A big memory**: 300 earlier, unrelated facts already in the notes file
  and in Memro, so B has to find the one that matters.
- **Another repo**: the rule is about the team's POS devices, and B works in a
  second repo of the same team.

## 6. Caveats

- One model (Haiku), small samples (5 relays per trap and arm).
- B works in the same checkout as A, which favours the notes file.
- "A wrote it down" is a keyword match on `NOTES.md` or the bench agent's
  memories, so it is approximate.
