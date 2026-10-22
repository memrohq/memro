# 0021. Cloudflare Credits for Embedding-Model Evaluation (benchmark only)
Date: 2026-10-23 · Status: Accepted

## Context
Memro has $5,000 of Cloudflare startup credits. They cover Workers AI (capped at
$2,500 on our tier) and R2, but not AI Gateway, and they expire one year after
they were issued. LongMemEval runs show retrieval finds the right session for
about 99% of questions, but 9 of the 22 failures in baseline-100 lacked part of
the exact evidence. So the embedding model is worth testing. The storage and
search engine is not: Memro stays self-hostable on Postgres + pgvector.

## Decision
- Use Workers AI only to compare embedding models, with everything else fixed:
  the same Memro server, Postgres + pgvector, hybrid search, questions, answer
  model and judge. Tooling lives in `bench/cf/` and never ships in Memro Cloud.
- No Vectorize. It would replace pgvector and can't run Memro's hybrid
  (vector + full-text + temporal) search, so its results wouldn't apply to
  Memro. PR #38's Vectorize and synthetic-vector scripts are not adopted.
- Never substitute data. The relay refuses to start without credentials,
  rejects models it doesn't know (no aliasing), and caches only real vectors.
  The Oct 21 pilot was already invalidated once by fake vectors.
- Compare exact answer-bearing exchanges (evidence R@5/R@15, all evidence in
  the prompt), and QA accuracy only alongside them. Confirm a candidate on all
  500 questions before any production decision.
- OpenAI models stay the answerer and judge, so judging is independent of the
  embedding under test.

## Consequences
- If a Workers AI model matches or beats OpenAI, adopting it is a separate
  decision. It would mean re-embedding every memory (different dimensions), and
  the model is open-weight, so it could be self-hosted rather than tied to
  Cloudflare.
- R2 for datasets and AI Gateway are out of scope until there's a need. CI
  downloads LongMemEval from Hugging Face, pinned by revision and checksum.
