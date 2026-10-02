"""Publish only trusted, checked commits; immutable versions and a rolling nightly."""
import glob
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from release_notes import LANGUAGES, render_notes

VERSION_RE = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?\Z")


def validate_version(value):
    if not VERSION_RE.fullmatch(value):
        raise ValueError("Invalid VERSION")
    return value


def api(path, method=None, body=None, missing=False):
    args = ["gh", "api", path]
    if method:
        args += ["--method", method]
    if body is not None:
        args += ["--input", "-"]
    result = subprocess.run(args, input=json.dumps(body) if body is not None else None,
                            text=True, capture_output=True)
    if result.returncode:
        if missing and "HTTP 404" in result.stderr:
            return None
        raise RuntimeError(result.stderr.strip())
    return json.loads(result.stdout) if result.stdout.strip() else None


def release_action(existing):
    if existing is None:
        return "create"
    return "resume" if existing["draft"] else "skip"


def main():
    repo = os.environ["GITHUB_REPOSITORY"]
    sha = os.environ["SOURCE_SHA"]
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo) or not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("Invalid repository or commit")
    version = validate_version(Path("VERSION").read_text().strip())
    change_notes = render_notes(version)
    tag = "v" + version
    is_tag = os.environ.get("GITHUB_REF_TYPE") == "tag"
    if is_tag and os.environ.get("GITHUB_REF_NAME") != tag:
        raise ValueError("Release tag must match VERSION")
    prefix = f"repos/{repo}"
    if not is_tag:
        head = api(prefix + "/git/ref/heads/master")["object"]["sha"]
        if head != sha:
            print("Skipping stale commit; master has advanced.")
            return
    assets = sorted(glob.glob("release/*.tar.gz")) + ["release/SHA256SUMS"]
    if len(assets) != 3 or any(not Path(p).is_file() for p in assets):
        raise ValueError("Missing release assets")
    subprocess.run(["sha256sum", "-c", "SHA256SUMS"], cwd="release", check=True)
    base_notes = (f"Commit: `{sha}`\n\nLinux amd64 and arm64 portable binaries, with SHA256SUMS. "
                  "Run `./nodesweep -version` to inspect build metadata.\n\n"
                  "Validate on a test VPS before use. Cleanup permanently removes archived logs "
                  "inside local allowlists; always review the preview. See README and SECURITY.md.\n")
    base_notes += "\n" + change_notes
    existing = api(prefix + "/releases/tags/" + tag, missing=True)
    action = release_action(existing)
    if action == "create":
        # A pre-existing tag must identify this checked source, including annotated tags.
        ref = api(prefix + "/git/ref/tags/" + tag, missing=True)
        if ref:
            obj = ref["object"]
            while obj["type"] == "tag":
                obj = api(prefix + "/git/tags/" + obj["sha"])["object"]
            if obj["type"] != "commit" or obj["sha"] != sha:
                raise ValueError("Existing version tag points to a different commit")
        if ref is None:
            api(prefix + "/git/refs", "POST", dict(ref="refs/tags/" + tag, sha=sha))
        existing = api(prefix + "/releases", "POST", dict(tag_name=tag, target_commitish=sha,
                       name="NodeSweep " + version, body=base_notes, draft=True,
                       prerelease="-" in version))
    if action != "skip":
        ref = api(prefix + "/git/ref/tags/" + tag)
        obj = ref["object"]
        while obj["type"] == "tag":
            obj = api(prefix + "/git/tags/" + obj["sha"])["object"]
        if obj["sha"] != sha:
            raise ValueError("Draft version belongs to a different commit")
        subprocess.run(["gh", "release", "upload", tag, *assets, "--clobber", "--repo", repo], check=True)
        api(prefix + "/releases/" + str(existing["id"]), "PATCH", dict(draft=False))
    else:
        print(f"Keeping published {tag} assets unchanged.")
        # Backfill missing translations without changing the original build metadata or assets.
        original_body = existing.get("body") or ""
        if not all("## " + label + "\n" in original_body for _, label in LANGUAGES):
            api(prefix + "/releases/" + str(existing["id"]), "PATCH",
                dict(body=original_body.rstrip() + "\n\n" + change_notes))
    if is_tag:
        return
    # Only the nightly tag is deliberately movable. No version release is overwritten.
    ref = api(prefix + "/git/ref/tags/nightly", missing=True)
    if ref:
        api(prefix + "/git/refs/tags/nightly", "PATCH", dict(sha=sha, force=True))
    else:
        api(prefix + "/git/refs", "POST", dict(ref="refs/tags/nightly", sha=sha))
    nightly = api(prefix + "/releases/tags/nightly", missing=True)
    notes = "Rolling development build. Updated when a new master commit passes CI; no scheduled rebuilds.\n\n" + base_notes
    if nightly is None:
        nightly = api(prefix + "/releases", "POST", dict(tag_name="nightly", name="NodeSweep Nightly",
                      body=notes, draft=True, prerelease=True, make_latest="false"))
    subprocess.run(["gh", "release", "upload", "nightly", *assets, "--clobber", "--repo", repo], check=True)
    api(prefix + "/releases/" + str(nightly["id"]), "PATCH", dict(name="NodeSweep Nightly", body=notes,
        draft=False, prerelease=True, make_latest="false"))


if __name__ == "__main__":
    main()
