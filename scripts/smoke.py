#!/usr/bin/env python3
"""Real standalone + two-agent smoke test; creates only temporary fixture files."""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "./nodesweep").resolve())
processes = []
with tempfile.TemporaryDirectory(prefix="nodesweep-smoke-") as directory:
    base = Path(directory)
    logs = base / "logs"
    logs.mkdir()
    (logs / "fixture.log.1").write_bytes(b"archived log\n" * 4096)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    token = os.urandom(32).hex()
    address = f"http://127.0.0.1:{port}"

    def start(name, config):
        path = base / (name + ".json")
        path.write_text(json.dumps(config))
        path.chmod(0o600)
        process = subprocess.Popen(
            [binary, "-config", str(path)],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        processes.append(process)

    def api(path, body=None):
        request = urllib.request.Request(
            address + "/api/" + path,
            data=json.dumps(body).encode() if body is not None else None,
            headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
        )
        with urllib.request.urlopen(request, timeout=10) as response:
            return json.load(response)

    def wait_for(check, timeout=30):
        deadline = time.monotonic() + timeout
        last = None
        while time.monotonic() < deadline:
            try:
                value = check()
                if value:
                    return value
            except Exception as error:
                last = error
            time.sleep(0.2)
        raise AssertionError(f"Timed out: {last}")

    try:
        start("hub", {
            "mode": "standalone", "listen": f"127.0.0.1:{port}",
            "data": str(base / "state.db"), "adminToken": token,
            "cleanupRoots": [str(logs)], "scanRoots": [str(logs)],
        })
        wait_for(lambda: api("nodes"))
        remote_ids = []
        for i in range(2):
            node = api("nodes", {"name": f"smoke-agent-{i}"})
            remote_ids.append(node["node"]["id"])
            start(f"agent-{i}", {
                "mode": "agent", "hub": address,
                "node": node["node"]["id"], "token": node["token"],
                "cleanupRoots": [str(logs)], "scanRoots": [str(logs)],
            })
        def connected():
            nodes = api("nodes")
            return len(nodes) == 3 and all(n["metrics"]["host"] for n in nodes)
        wait_for(connected)
        task = api("tasks", {"node": remote_ids[0], "request": {"kind": "scan", "path": str(logs)}})
        def finished():
            result = api("tasks/" + task["id"])
            return result if result["status"] in ("succeeded", "failed") else None
        result = wait_for(finished)
        assert result["status"] == "succeeded", result
        assert result["result"]["tree"]["bytes"] > 0, result
        assert result["result"]["files"] == 1, result
        print("PASS: standalone, two outbound agents, live metrics and remote scan")
    finally:
        for process in processes:
            process.terminate()
        for process in processes:
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
