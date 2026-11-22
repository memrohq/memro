"""Dumps every issue and pull request of a repo with its comments, as the GitHub
connector reads them (the issues API and issue comments), to data/<repo>.json.

    python3 bench/github_history/fetch.py memrohq/memro
"""
import json
import pathlib
import subprocess
import sys


def gh(path):
    out = subprocess.run(["gh", "api", "--paginate", "--slurp", path], capture_output=True, text=True, check=True)
    return [x for page in json.loads(out.stdout) for x in page]


repo = sys.argv[1]
issues = gh(f"repos/{repo}/issues?state=all&per_page=100")
for i in issues:
    i["_repo"] = repo
    i["_comments"] = gh(f"repos/{repo}/issues/{i['number']}/comments?per_page=100") if i["comments"] else []
out = pathlib.Path(__file__).parent / "data" / f"{repo.replace('/', '__')}.json"
out.parent.mkdir(exist_ok=True)
out.write_text(json.dumps(issues))
print(f"{len(issues)} issues and PRs, {sum(len(i['_comments']) for i in issues)} comments -> {out}")
