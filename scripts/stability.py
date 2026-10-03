#!/usr/bin/env python3
"""Exercise an isolated local NodeSweep; never connect to an existing service."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import socket
import subprocess
import tempfile
import time
import urllib.request


def bounded_int(low, high):
    def parse(value):
        number = int(value)
        if not low <= number <= high:
            raise argparse.ArgumentTypeError(f"expected {low}..{high}")
        return number
    return parse


def verify_scan(task, limit):
    result = task.get("result") or {}
    if (task.get("status") != "succeeded" or not result.get("truncated")
            or result.get("reason") != "entries" or result.get("files") != limit):
        raise RuntimeError("entry budget assertion failed")
    if len(json.dumps(result).encode()) > 8 << 20:
        raise RuntimeError("result payload assertion failed")


def run(binary, files, rounds, seconds):
    hasher = hashlib.sha256()
    with open(binary, "rb") as executable:
        for chunk in iter(lambda: executable.read(1 << 20), b""):
            hasher.update(chunk)
        digest = hasher.hexdigest()
    report = {"schema": "nodesweep.stability.v1", "architecture": platform.machine(),
              "binarySha256": digest, "requestedRounds": rounds, "requestedSeconds": seconds,
              "fixtureFiles": files, "roundsCompleted": 0, "peakRssKiB": None,
              "peakFdCount": None, "maxScanSeconds": 0, "passed": False}
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="nodesweep-stability-") as directory:
        base = Path(directory)
        fixture = base / "fixture"
        fixture.mkdir(mode=0o700)
        # Empty files exercise directory cardinality without allocating log data.
        for i in range(files):
            (fixture / f"{i:07d}.log").touch(mode=0o600)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        token = os.urandom(32).hex()
        config = base / "config.json"
        config.write_text(json.dumps({"mode": "standalone", "listen": f"127.0.0.1:{port}",
                                     "data": str(base / "state.db"), "adminToken": token,
                                     "cleanupRoots": [], "scanRoots": [str(fixture)],
                                     "scanBudget": {"entries": 1000, "seconds": 10,
                                                    "treeBytes": 8 << 20, "pauseMillis": 50}}))
        config.chmod(0o600)
        process = subprocess.Popen([binary, "-config", str(config)], cwd=base,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

        def sample():
            if process.poll() is not None:
                raise RuntimeError("isolated server exited")
            try:
                status = Path(f"/proc/{process.pid}/status").read_text()
                for line in status.splitlines():
                    if line.startswith("VmRSS:"):
                        report["peakRssKiB"] = max(report["peakRssKiB"] or 0, int(line.split()[1]))
                count = len(list(Path(f"/proc/{process.pid}/fd").iterdir()))
                report["peakFdCount"] = max(report["peakFdCount"] or 0, count)
            except (OSError, ValueError):
                pass  # Unavailable measurements remain null, never synthetic zero.

        def api(path, body=None):
            sample()
            request = urllib.request.Request(
                f"http://127.0.0.1:{port}/api/{path}",
                data=json.dumps(body).encode() if body is not None else None,
                headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
            # Ignore ambient proxy settings; this process is bound to loopback only.
            with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=5) as response:
                raw = response.read((12 << 20) + 1)
            if len(raw) > 12 << 20:
                raise RuntimeError("API payload assertion failed")
            return json.loads(raw)

        def wait(check, timeout):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                result = check()
                if result:
                    return result
                time.sleep(0.05)
            raise RuntimeError("isolated operation timed out")

        def ready():
            try:
                return api("nodes")
            except (OSError, urllib.error.URLError):
                return None

        def scan():
            return api("tasks", {"node": "local", "request": {"kind": "scan", "path": str(fixture)}})

        def terminal(task):
            result = api("tasks/" + task["id"])
            return result if result["status"] in ("succeeded", "failed", "interrupted") else None

        try:
            wait(ready, 15)
            soak_start = time.monotonic()
            while report["roundsCompleted"] < rounds or time.monotonic() - soak_start < seconds:
                scan_start = time.monotonic()
                task = scan()
                verify_scan(wait(lambda: terminal(task), 15), 1000)
                report["maxScanSeconds"] = max(report["maxScanSeconds"], time.monotonic() - scan_start)
                report["roundsCompleted"] += 1
                if seconds:
                    time.sleep(1)
            task = scan()

            def progressed():
                current = api("tasks/" + task["id"])
                if current["status"] not in ("pending", "running"):
                    raise RuntimeError("scan finished before cancellation assertion")
                return current if (current.get("progress") or {}).get("visited", 0) >= 128 else None

            wait(progressed, 5)
            cancelled_at = time.monotonic()
            api("tasks/" + task["id"] + "/cancel", {})
            cancelled = wait(lambda: terminal(task), 5)
            if (cancelled["status"] != "failed" or not cancelled.get("cancelRequested")
                    or (cancelled.get("result") or {}).get("reason") != "cancelled"
                    or not (cancelled.get("result") or {}).get("tree")):
                raise RuntimeError("cancellation assertion failed")
            report["cancelSeconds"] = time.monotonic() - cancelled_at
            # Cancellation must release admission so another scan can run.
            task = scan()
            verify_scan(wait(lambda: terminal(task), 15), 1000)
            report["passed"] = True
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
    report["elapsedSeconds"] = round(time.monotonic() - started, 3)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=lambda value: str(Path(value).resolve()))
    parser.add_argument("--files", type=bounded_int(2000, 1000000), default=2000)
    parser.add_argument("--rounds", type=bounded_int(1, 10000), default=3)
    parser.add_argument("--seconds", type=bounded_int(0, 86400), default=0,
                        help="minimum soak duration after fixture creation; default 0")
    args = parser.parse_args()
    try:
        report = run(args.binary, args.files, args.rounds, args.seconds)
    except Exception as error:
        # Raw OS/network errors can include private paths; do not export them.
        print(json.dumps({"schema": "nodesweep.stability.v1", "passed": False,
                          "failureType": type(error).__name__}))
        return 1
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
