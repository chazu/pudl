#!/usr/bin/env python3
"""Read the walkthrough's supplied inventory using mu's NDJSON protocol."""

import json
from pathlib import Path
import sys


def dispatch(request):
    method = request.get("method")
    if method == "discover":
        return {
            "name": "git-inventory",
            "version": "0.1.0",
            "protocol_version": 1,
            "consumes": [],
            "produces": [],
            "capabilities": ["discover", "observe"],
        }
    if method == "observe":
        config = request["target"]["config"]
        records = json.loads(Path(config["inventory"]).read_text(encoding="utf-8"))
        if not isinstance(records, list) or not all(isinstance(r, dict) for r in records):
            raise ValueError("inventory must be a JSON array of records")
        return {"current": {"records": records}}
    return {"error": f"unsupported method: {method}"}


for line in sys.stdin:
    if not line.strip():
        continue
    try:
        response = dispatch(json.loads(line))
    except Exception as error:
        response = {"error": str(error)}
    print(json.dumps(response), flush=True)
