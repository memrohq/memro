# LoCoMo on Memro: locomo-openai

Held-out evaluation only (summary scores; no per-question results are kept). Embedding model: `text-embedding-3-small`; answers: `gpt-4o-mini`; judge: gpt-4o-mini. Category 5 (adversarial) excluded.

| Category | n | Accuracy | Evidence R@5 | Evidence R@15 | Evidence in prompt |
|---|---|---|---|---|---|
| 1 (multi-hop) | 282 | 54.3% | 33.3% | 53.9% | 65.9% |
| 2 (temporal) | 321 | 47.7% | 73.0% | 82.6% | 89.6% |
| 3 (open-domain) | 96 | 32.3% | 30.6% | 44.0% | 56.6% |
| 4 (single-hop) | 841 | 81.8% | 70.9% | 85.1% | 94.1% |
| **overall** | 1540 | 66.6% | 62.1% | 76.5% | 85.8% |

Cost: $1.1722
