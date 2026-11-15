# 0033. Thresholds use similarity, not the fused score; sentiment and negation match whole words
Date: 2026-11-15 · Status: Accepted

## Context
Writing Memro's search and contradiction rules as equations for the research
papers (memro.co/research, MR-2026-03 and MR-2026-05) exposed two problems in
the code.

**`min_score` could not reject irrelevant results.** Hybrid search fuses the
semantic and keyword lists with Reciprocal Rank Fusion and normalises by the
best possible sum (`fuseRRF`, k = 60):

    score(d) = min(1, (k+1)/2 · Σ_L 1/(k + rank_L(d)))

The score depends only on ranks. A memory found by one list at rank n scores
(k+1)/(2(k+n)), which is above 0.4 for every n ≤ 16, however unrelated it is.
The SDKs' `min_score` filtered on this score, so `min_score=0.4` kept the top
16 candidates of any query. The verification suite's distractor test
(`bench/eval_suite.py`, an unrelated query with `min_score=0.4`) failed for
this reason: five unrelated results came back.

**Sentiment and negation words matched as substrings.** "like" matched
"likely" and "dislike" (so "dislike" counted as both positive and negative),
"bad" matched "badge", "best" matched "bestseller". On `main`, "Picked up my
badge at the front desk." against "Good coffee at the front desk." scored 0.70,
enough to mark one of them superseded in search; "The bestseller list came out
today." against "The worst traffic was on Monday." scored 0.60.

## Decision
1. **Search results carry `similarity`**, the cosine similarity between the
   query embedding and the memory's embedding (`1 − (embedding <=> query)`),
   computed when results are loaded; absent when either has no embedding.
   `score` is unchanged and still orders results.
2. **The SDKs apply `min_score` to `similarity`** (Python `min_score`, JS
   `minScore`, Go `MinScore`), falling back to `score` only for a result with
   no similarity (keyword mode, or a memory not yet embedded).
3. **Sentiment and negation are matched as whole words**, with their
   inflections listed explicitly (`loved`, `hating`, `preferred`…). The stem
   lists stay, only to ignore sentiment words when deciding whether two
   statements make the same claim.

## Alternatives considered
- **Threshold on the server (`min_score` query parameter).** Cleaner for API
  users, but the SDKs already filter client-side; adding `similarity` fixes
  every SDK and leaves API users free to choose. A server parameter can come
  later.
- **Make `score` the similarity.** Breaks the ordering that hybrid search
  relies on (RRF is what lets keyword-only matches compete), and changes the
  meaning of a field clients already read.
- **Normalise RRF differently.** Any rank-based normalisation has the same
  property: it measures position, not relevance.
- **Keep substring matching and add exceptions.** Each new word would need its
  own exceptions; whole words remove the class of bug.

## Consequences
- `min_score` now means "at least this similar to the query". Callers who
  tuned `min_score` against the old rank-based score may get fewer results;
  typical `text-embedding-3-small` similarities are lower than the old scores.
- One extra `LEFT JOIN memory_embeddings` per search, on the result IDs only.
- Graph-expansion results (off by default) carry no similarity yet, so
  `min_score` falls back to their score.
- Sentiment flips are detected only for listed forms; an unlisted inflection
  is missed rather than mis-matched.
- Tests: `TestSearchReturnsSimilarityForThresholds` (an unrelated query keeps
  a rank-based score ≥ 0.4 while every similarity is below 0.4),
  `TestSentimentAndNegationAreWholeWords` (the pairs above now score 0; real
  flips still ≥ 0.7), and SDK tests. The evolution benchmark stays at 0/9
  unresolved updates, 0/2 duplicates left and 0/7 false updates.
