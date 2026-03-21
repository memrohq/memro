import sys
import json
import os

LOG_FILE = "/Users/freya/Documents/work/hackit/memrohq/mcp_traffic.log"

def log(direction, data):
    try:
        with open(LOG_FILE, "a") as f:
            f.write(f"[{direction}] {data.strip()}\n")
    except:
        pass

# Setup subprocess to run the real server
import subprocess
env = os.environ.copy()
# Ensure we use the correct python
cmd = ["/Users/freya/Documents/work/hackit/memrohq/.venv/bin/python", "/Users/freya/Documents/work/hackit/memrohq/memro-mcp/src/memro_mcp/server.py"]

proc = subprocess.Popen(
    cmd,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=sys.stderr, # Forward stderr to see it in console
    env=env,
    bufsize=0,
    text=True
)

import threading

def forward_stdin():
    for line in sys.stdin:
        log("IN", line)
        proc.stdin.write(line)
        proc.stdin.flush()

def forward_stdout():
    for line in proc.stdout:
        log("OUT", line)
        sys.stdout.write(line)
        sys.stdout.flush()

t1 = threading.Thread(target=forward_stdin)
t2 = threading.Thread(target=forward_stdout)
t1.start()
t2.start()
t1.join()
t2.join()
