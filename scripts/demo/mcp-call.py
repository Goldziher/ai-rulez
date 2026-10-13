#!/usr/bin/env python3
"""Tiny MCP client for the README demo: initialize, then one tools/call, print the text.

Usage: mcp-call.py <tool> <json-arguments> [max-lines]
Starts `ai-rulez mcp --serve-skills --read-only`, completes the initialize/initialized
handshake, calls the tool and prints the text content (first max-lines lines).
"""

import json
import subprocess
import sys

tool, args = sys.argv[1], json.loads(sys.argv[2])
limit = int(sys.argv[3]) if len(sys.argv) > 3 else 8

proc = subprocess.Popen(
    ["ai-rulez", "mcp", "--serve-skills", "--read-only"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.DEVNULL,
    text=True,
)


def send(msg):
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()


def recv(req_id):
    for line in proc.stdout:
        msg = json.loads(line)
        if msg.get("id") == req_id:
            return msg
    raise SystemExit("server closed before answering")


send(
    {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "demo", "version": "1"}},
    }
)
recv(1)
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
send({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": tool, "arguments": args}})
result = recv(2)
proc.stdin.close()
proc.wait(timeout=5)

if "error" in result:
    raise SystemExit(result["error"]["message"])
text = "\n".join(c.get("text", "") for c in result["result"]["content"])
try:
    data = json.loads(text)
except ValueError:
    data = None
if isinstance(data, dict) and "results" in data:
    text = "\n".join(f"{r['name']}: {r['description'].split('. ')[0]}" for r in data["results"])
elif isinstance(data, dict) and "content" in data:
    text = data["content"].split("---\n", 2)[-1]
lines = [ln for ln in text.splitlines() if ln.strip()]
print("\n".join(lines[:limit]))
if len(lines) > limit:
    print("...")
