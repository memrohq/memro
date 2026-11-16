"""Accuracy against answer-prompt tokens per question, one row per judged run.

    python tokens.py runs/full-ctx-100 runs/arm-memro-100 runs/budget-2000-100

The first run is the reference for the "x fewer tokens" column. Needs runs made
after per-row prompt_tokens was recorded; older rows are skipped.
"""
import pathlib
import statistics
import sys

from compare import load_run


def main(paths):
    rows_out = []
    for path in paths:
        rows, labels, meta = load_run(pathlib.Path(path))
        ids = [i for i, r in rows.items() if i in labels and r.get("prompt_tokens")]
        if not ids:
            print(f"{path}: no judged rows with prompt_tokens, skipped", file=sys.stderr)
            continue
        acc = sum(bool(labels[i]) for i in ids) / len(ids)
        tokens = statistics.mean(rows[i]["prompt_tokens"] for i in ids)
        rows_out.append((pathlib.Path(path).name, acc, tokens, len(ids)))
    if not rows_out:
        sys.exit("nothing to report")
    ref = rows_out[0][2]
    print("| run | accuracy | prompt tokens / question | x fewer than first | n |")
    print("|---|---|---|---|---|")
    for name, acc, tokens, n in rows_out:
        print(f"| {name} | {acc:.1%} | {tokens:,.0f} | {ref / tokens:.1f}x | {n} |")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    main(sys.argv[1:])
