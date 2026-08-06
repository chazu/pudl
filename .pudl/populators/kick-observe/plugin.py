#!/usr/bin/env python3
"""Repository-local mu plugin used by the PUDL kick-the-tires runbook."""

import json
import os
import pathlib
import sys


SECRET_VALUE = "kick-token-value"


def emit(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)


def record(method, target=""):
    path = os.environ.get("PUDL_KICK_SENTINEL")
    if not path:
        return
    sentinel = pathlib.Path(path)
    sentinel.parent.mkdir(parents=True, exist_ok=True)
    with sentinel.open("a", encoding="utf-8") as stream:
        stream.write(f"{method} {target}\n")


def target_config(request):
    return request.get("target", {}).get("config", {})


def observe(request):
    target = request.get("target", {}).get("name", "")
    record("observe", target)
    config = target_config(request)
    role = config.get("role", "unknown")
    if os.environ.get("PUDL_KICK_FAIL_ROLE") == role:
        return {"error": f"scripted observation failure for {role}"}
    if role == "producer":
        return {"current": {"records": [{
            "_schema": "kicktires.thing",
            "name": "one",
            "value": config.get("value", "ready"),
            "private": "not-authorized",
        }]}}
    if role == "consumer":
        return {"current": {"records": [{
            "_schema": "kicktires.consumer",
            "name": "consumer",
            "bound": config.get("bound", "missing"),
        }]}}
    state = pathlib.Path(config.get("state", f"state/{role}"))
    clean = state.exists()
    return {"current": {"resources": [{
        "resource": f"Kick/{role}",
        "exists": clean,
        "matches": clean,
    }]}}


def action_for(request):
    target = request.get("target", {})
    config = target.get("config", {})
    role = config.get("role", "unknown")
    state = config.get("state", f"state/{role}")
    variant = os.environ.get("PUDL_KICK_PLAN_VARIANT", "stable")
    sealed_inputs = dict(target.get("sealed_inputs", {}))
    sealed_input_modes = dict(target.get("sealed_input_modes", {}))
    sealed_outputs = dict(target.get("sealed_outputs", {}))
    sealed_output_modes = dict(target.get("sealed_output_modes", {}))
    drop = os.environ.get("PUDL_KICK_DROP_CLAIMS")
    if drop == "input":
        sealed_inputs = {}
        sealed_input_modes = {}
    if drop == "output":
        sealed_outputs = {}
        sealed_output_modes = {}
    extra = os.environ.get("PUDL_KICK_EXTRA_CLAIM")
    if extra == "input":
        sealed_inputs["UNDECLARED"] = "kicksecret:undeclared"
        sealed_input_modes["UNDECLARED"] = "env"
    if extra == "output":
        sealed_outputs["UNDECLARED"] = "kicksecret:undeclared"
        sealed_output_modes["UNDECLARED"] = "overwrite"
    if role == "sealed-producer":
        script = (
            f'mkdir -p "$(dirname {state})" && : > "{state}" && '
            f'printf %s "{SECRET_VALUE}" > "$MU_SEALED_OUT_DIR/TOKEN"'
        )
    elif role == "sealed-consumer":
        script = (
            f'test "$TOKEN" = "{SECRET_VALUE}" && '
            f'mkdir -p "$(dirname {state})" && : > "{state}"'
        )
    else:
        script = f'mkdir -p "$(dirname {state})" && : > "{state}"'
    return {
        "id": f"apply-{role}",
        "command": ["/bin/sh", "-c", script],
        "inputs": {},
        "outputs": [],
        "env": {"KICK_PLAN_VARIANT": variant},
        "sealed_inputs": sealed_inputs,
        "sealed_input_modes": sealed_input_modes,
        "sealed_outputs": sealed_outputs,
        "sealed_output_modes": sealed_output_modes,
        "impure": True,
    }


def plan(request):
    target = request.get("target", {}).get("name", "")
    record("plan", target)
    action = action_for(request)
    actions = [action]
    if os.environ.get("PUDL_KICK_DUPLICATE_OUTPUT_CLAIM"):
        duplicate = dict(action)
        duplicate["id"] = f'{action["id"]}-duplicate'
        actions.append(duplicate)
    return {"actions": actions, "declared_outputs": {}}


def secret_store_path():
    return pathlib.Path(os.environ.get("PUDL_KICK_SECRET_STORE", "secrets.json"))


def load_secrets():
    path = secret_store_path()
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def store_secret(request):
    record("store_secret", request.get("secret_ref", ""))
    path = secret_store_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    secrets = load_secrets()
    ref = request.get("secret_ref", "")
    mode = request.get("secret_mode", "overwrite")
    if mode == "create" and ref in secrets:
        return {"error": f"secret {ref} already exists"}
    if mode == "create_if_absent" and ref in secrets:
        return {}
    secrets[ref] = request.get("secret_value", "")
    path.write_text(json.dumps(secrets, sort_keys=True), encoding="utf-8")
    return {}


def resolve_secret(request):
    ref = request.get("secret_ref", "")
    record("resolve_secret", ref)
    secrets = load_secrets()
    if ref not in secrets:
        return {"error": f"secret {ref} not found"}
    return {"value": secrets[ref]}


def dispatch(request):
    method = request.get("method")
    if method == "discover":
        record("discover")
        return {
            "name": "kicksecret",
            "version": "0.1.0",
            "protocol_version": 1,
            "consumes": [],
            "produces": [],
            "capabilities": [
                "discover", "plan", "observe", "resolve_secret", "store_secret"
            ],
        }
    if method == "observe":
        return observe(request)
    if method == "plan":
        return plan(request)
    if method == "resolve_secret":
        return resolve_secret(request)
    if method == "store_secret":
        return store_secret(request)
    return {"error": f"unknown method: {method}"}


for line in sys.stdin:
    if not line.strip():
        continue
    try:
        emit(dispatch(json.loads(line)))
    except Exception as error:  # The protocol returns errors instead of crashing.
        emit({"error": str(error)})
