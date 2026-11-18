# 2026-11-19: Relay — a rule buried among 3,000 facts

Written before the run. Plan, predictions and pass bar are fixed here; results
are added below them without changing them.

- **Harness**: `bench/relay/run.py --setting huge --plant 25|50|75`, branch `bench/huge-relay`
- **Agents**: Claude Code, `--model haiku`, one at a time
- **Server**: a local build of `main` (production caps an unclaimed agent at
  1,000 memories; see `docs/roadmap.md`), embeddings `text-embedding-3-small`
  as in production

## 1. Question

The 300-fact relay (journal 2026-11-17, §8) was a tie: Memro with hooks 10/10,
a notes file 9/10. There the agent reads the notes file whole, and A's rule
landed at its end. A LinkedIn reader asked for the controls this run adds: the
rule at fixed depths in a memory too big to read whole, about 50 trials per
cell, and a Wilson interval for every arm.

## 2. Setup

Only agent B runs. Before it starts, R1's and R2's rules sit at 25%, 50% or
75% of 3,000 earlier facts (100 of them earlier rules for this repo) in every
arm's store, so all arms hold the same facts in the same order:

| Arm | Where the facts are |
|---|---|
| none | nowhere (the floor) |
| notes | `NOTES.md`, one line each; CLAUDE.md says to read it first. Read shows the first 2,000 lines, so 75% (line ~2,250) is past it |
| automem | Claude Code's own auto-memory: a file per fact and a `MEMORY.md` index. Claude Code loads the index's first 200 lines (checked 2026-11-19: "2800 of 3000 lines were cut off") |
| memro-hooks | Memro with `memro connect`'s hooks; the rules saved as the rule hook saves them (`Rule for shop: …`) |
| memro-plain | the same hooks, but the trap rules saved as plain memories, so only search can find them |

In Memro the rules are saved among the earlier facts at the same depth, so
they are no newer than the rest. B's task is the indirect one (it repeats none
of the rule's words). Each row also logs `b_saw_rule`: the rule's own words
came back to B in a tool result or hook output.

**Cells**: R1 + R2 × 25 runs = 50 trials per arm and depth; none 50 trials in
all. 4 arms × 3 depths × 50 + 50 = **650 B sessions**, about 6 hours at ~30 s
each and about $8.

## 3. Predictions (before running)

- **notes**: passes at 25% and 50% (the file's first 2,000 lines show), dips
  at 75%, unless B pages on through the file.
- **automem**: below notes at every depth, since only 200 index lines load and
  B has to search the memory folder itself.
- **memro-hooks**: flat across depths, at or near 50/50. Its risk is the
  hook's 20-rule cap: with 102 rules for this repo, the trap rule reaches B
  only if the rule listing ranks it among the 20.
- **memro-plain**: flat across depths but lower than memro-hooks; at 300 facts
  Memro's tools alone avoided the trap 6/10 and 8/10.
- **none**: about a third, as before (5/15).

## 4. Pass bar

A claim that Memro beats a notes file needs, at some depth, memro-hooks above
notes with Fisher p < 0.01 (for example 50/50 against 41/50 or fewer). Every
cell is reported with its Wilson 95% interval and its p against notes at the
same depth, including cells where Memro loses. A tie is published as a tie.

## 5. Changed before the run

The first smoke test (one relay per arm, R1, rule at 50%) found Memro's hook
showing 20 of the repo's 102 rules, none of them the trap's, so both Memro
arms missed. That limit was already written down as a shortcut (ADR 0035), so
it was fixed before the long run, not after seeing it: the server gained
`GET /rules?repo=…` (all of a repo's hook-saved rules, up to 200) and the hook
uses it. With the fix, a second smoke test (R1 + R2, rule at 50%): notes 2/2,
memro-hooks 2/2, memro-plain 1/2, automem 0/2. The memro-hooks prediction
above was written with the cap in mind and is left as it was.

## 6. Known limits

- One model (Haiku) and two traps.
- The notes arm's dip at 75% would be truncation by Read, not attention lost
  in the middle; 25% vs 50% is the lost-in-the-middle comparison.
- Depths run one after another (25%, then 50%, then 75%), arms interleaved
  within each, so a slow model hour can shift one depth but not one arm.
- The hooks arms are told by CLAUDE.md to recall first, and so is notes to
  read its file and automem to check its memory: same instruction, three stores.
