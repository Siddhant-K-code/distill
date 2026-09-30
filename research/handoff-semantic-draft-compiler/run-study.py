#!/usr/bin/env python3
"""Frozen launcher for the Handoff semantic-draft compiler study."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import stat
import subprocess
import tempfile
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


COPILOT_VERSION = "GitHub Copilot CLI 1.0.90-5."
COPILOT_SHA256 = "a21a8624194aabc709cdd8ec564b653efb0d771057607b040047e9a1c569a25b"
MODEL = "gpt-5.6-sol"
PROMPT_SHA256 = "f348b76c98ed70a09835e17c14d9b281da113dc45ab142a1a382829899f9838d"
PROTOCOL_SHA256 = "8d76e52cd88ee5896add3f76f9aa3afe1a9044ecf019b56a94e9fc0d35039ea5"
SCHEMA_SHA256 = "7f05117827b50c7ddec7ac33fbab9b58194aa272a698588e870f93fec34ce5a6"
BASELINE = "6aaefbb53acc5cb177d617292f96cd3b87f693ba"

CONDITIONS = {
    "positive": {
        "fixture": "testdata/handoff-v0/retry-policy",
        "request_id": "94730d11355f823740c1a4d9a7ff73c8841bc6c0038e8f6ac1c0d57829a75487",
        "request_sha256": "cd32864f9e42b44e0341bb9199dabb28a032103b8e2d9e9b4f4c19d61d3aea38",
        "conversation_sha256": "94a8d1951427b388d37919fecee4d68ae2f0d4398199b29214ce5230ef74077a",
        "document_path": "docs/runbook/retries.md",
        "document_sha256": "21f128d20f43ebeaa8a37a8519363103ea8cf4d57b126c7643879bfc07fbea70",
    },
    "negative": {
        "fixture": "testdata/handoff-v0/brainstorming",
        "request_id": "eb613088a5e48b03b2f1cac9c47a6a6338f02dc859e3f3e6afd0b686cb36df47",
        "request_sha256": "45d333595c146fcd8ade578f6015258dda8c57275b75352d63ca08f8c89fb4d2",
        "conversation_sha256": "a706e599c3a4b29b8c7e289e21ed2b9fd8376e4e6c7ca69a7dd25ec308d05326",
        "document_path": "docs/architecture/cache.md",
        "document_sha256": "12f10d33f8c077c0611e58bcdee398c9d6e93641fbf91e62aa6a31a5cc64a4c7",
    },
}

RUN_ORDER = (
    ("positive-1", "positive", 1),
    ("negative-1", "negative", 1),
    ("positive-2", "positive", 2),
    ("negative-2", "negative", 2),
    ("positive-3", "positive", 3),
    ("negative-3", "negative", 3),
)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def file_manifest(root: Path, exclude_git: bool = False) -> dict[str, str]:
    manifest: dict[str, str] = {}
    for path in sorted(root.rglob("*")):
        relative = path.relative_to(root).as_posix()
        if exclude_git and (relative == ".git" or relative.startswith(".git/")):
            continue
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode):
            manifest[relative] = "symlink:" + os.readlink(path)
        elif stat.S_ISREG(info.st_mode):
            manifest[relative] = sha256_file(path)
        elif stat.S_ISDIR(info.st_mode):
            manifest[relative + "/"] = "directory"
        else:
            manifest[relative] = "special"
    return manifest


def manifest_changes(before: dict[str, str], after: dict[str, str]) -> list[str]:
    names = sorted(set(before) | set(after))
    return [name for name in names if before.get(name) != after.get(name)]


def command_output(command: list[str], cwd: Path | None = None) -> str:
    process = subprocess.run(
        command,
        cwd=cwd,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return process.stdout


def validate_frozen_inputs(repo: Path, copilot: Path) -> None:
    checks = {
        repo / "research/handoff-semantic-draft-compiler/prompt.txt": PROMPT_SHA256,
        repo / "research/handoff-semantic-draft-compiler/protocol.md": PROTOCOL_SHA256,
        repo / "research/handoff-semantic-draft-compiler/semantic-draft.schema.json": SCHEMA_SHA256,
    }
    for path, expected in checks.items():
        actual = sha256_file(path)
        if actual != expected:
            raise RuntimeError(f"frozen hash mismatch for {path.name}: {actual}")
    if sha256_file(copilot) != COPILOT_SHA256:
        raise RuntimeError("Copilot executable SHA-256 changed")
    version = command_output([str(copilot), "--version"]).splitlines()[0]
    if version != COPILOT_VERSION:
        raise RuntimeError(f"Copilot version changed: {version!r}")
    head = command_output(["git", "rev-parse", "HEAD"], cwd=repo).strip()
    if head != BASELINE:
        raise RuntimeError(f"repository baseline changed: {head}")
    for condition in CONDITIONS.values():
        fixture = repo / condition["fixture"]
        if sha256_file(fixture / "conversation.md") != condition["conversation_sha256"]:
            raise RuntimeError("conversation fixture changed")
        if sha256_file(fixture / condition["document_path"]) != condition["document_sha256"]:
            raise RuntimeError("document fixture changed")


def prepare_requests(repo: Path, execution_root: Path, distill: Path) -> dict[str, Any]:
    records: dict[str, Any] = {}
    canonical_root = execution_root / "canonical"
    canonical_root.mkdir(mode=0o700)
    for name, condition in CONDITIONS.items():
        fixture = repo / condition["fixture"]
        output = canonical_root / f"{name}-request"
        stdout = command_output(
            [
                str(distill),
                "handoff",
                "prepare",
                "--conversation",
                str(fixture / "conversation.md"),
                "--docs",
                str(fixture / "docs"),
                "--out",
                str(output),
            ],
            cwd=repo,
        )
        request_path = output / "handoff.request.json"
        request = json.loads(request_path.read_text(encoding="utf-8"))
        request_sha256 = sha256_file(request_path)
        if request["request_id"] != condition["request_id"]:
            raise RuntimeError(f"{name} request ID did not reproduce")
        if request_sha256 != condition["request_sha256"]:
            raise RuntimeError(f"{name} request SHA-256 did not reproduce")
        records[name] = {
            "path": str(output),
            "request_id": request["request_id"],
            "request_sha256": request_sha256,
            "prepare_stdout": stdout.strip(),
            "manifest": file_manifest(output),
        }
    return records


def verify_runtime(copilot: Path) -> None:
    if sha256_file(copilot) != COPILOT_SHA256:
        raise RuntimeError("Copilot executable SHA-256 changed before invocation")
    version = command_output([str(copilot), "--version"]).splitlines()[0]
    if version != COPILOT_VERSION:
        raise RuntimeError("Copilot version changed before invocation")


def execute_run(
    repo: Path,
    execution_root: Path,
    copilot: Path,
    prompt: str,
    canonical: dict[str, Any],
    run_name: str,
    condition_name: str,
    ordinal: int,
    order_position: int,
    repository_before: dict[str, str],
) -> dict[str, Any]:
    verify_runtime(copilot)
    run_root = execution_root / "runs" / run_name
    run_root.mkdir(parents=True, mode=0o700)
    metadata = run_root / "runtime"
    metadata.mkdir(mode=0o700)
    logs = metadata / "logs"
    logs.mkdir(mode=0o700)

    workspace = Path(tempfile.mkdtemp(prefix=f"distill-{run_name}-", dir="/private/tmp"))
    os.chmod(workspace, 0o700)
    request_copy = workspace / "request"
    shutil.copytree(canonical["path"], request_copy, copy_function=shutil.copy2)
    output = workspace / "output"
    output.mkdir(mode=0o700)

    request_before = file_manifest(request_copy)
    workspace_before = file_manifest(workspace)
    canonical_before = file_manifest(Path(canonical["path"]))
    session_id = str(uuid.uuid4())
    usage_path = metadata / "usage.json"
    stdout_path = metadata / "events.jsonl"
    stderr_path = metadata / "stderr.txt"
    command = [
        str(copilot),
        "-C",
        str(workspace),
        "--prompt",
        prompt,
        "--model",
        MODEL,
        "--session-id",
        session_id,
        "--allow-all-tools",
        "--disallow-temp-dir",
        "--log-dir",
        str(logs),
        "--output-format",
        "json",
        "--usage-output-file",
        str(usage_path),
        "--no-custom-instructions",
        "--disable-builtin-mcps",
        "--no-auto-update",
        "--no-ask-user",
        "--no-remote",
        "--no-color",
        "--stream",
        "off",
        "--available-tools",
        "view",
        "apply_patch",
    ]

    start_utc = utc_now()
    start = time.monotonic_ns()
    with stdout_path.open("wb") as stdout, stderr_path.open("wb") as stderr:
        process = subprocess.run(command, cwd=workspace, stdout=stdout, stderr=stderr, check=False)
    elapsed_ms = (time.monotonic_ns() - start) // 1_000_000
    end_utc = utc_now()

    request_after = file_manifest(request_copy)
    canonical_after = file_manifest(Path(canonical["path"]))
    workspace_after = file_manifest(workspace)
    repository_after = file_manifest(repo, exclude_git=True)
    request_changes = manifest_changes(request_before, request_after)
    canonical_changes = manifest_changes(canonical_before, canonical_after)
    repository_changes = manifest_changes(repository_before, repository_after)

    intended = set(workspace_before)
    intended.add("output/semantic-draft.json")
    trust_changes = sorted(set(workspace_after) - intended)
    output_files = sorted(
        path.relative_to(output).as_posix() for path in output.rglob("*") if path.is_file()
    )
    draft = output / "semantic-draft.json"
    retained_draft = run_root / "semantic-draft.json"
    if draft.is_file():
        shutil.copy2(draft, retained_draft)

    record: dict[str, Any] = {
        "run": run_name,
        "condition": condition_name,
        "ordinal": ordinal,
        "order_position": order_position,
        "runtime": "GitHub Copilot CLI",
        "runtime_version": "1.0.90-5",
        "runtime_sha256": COPILOT_SHA256,
        "model": MODEL,
        "model_selected_explicitly": True,
        "available_tools": ["view", "apply_patch"],
        "session_id": session_id,
        "start_time_utc": start_utc,
        "end_time_utc": end_utc,
        "cli_elapsed_ms": elapsed_ms,
        "exit_status": process.returncode,
        "output_files": output_files,
        "output_file_count": len(output_files),
        "draft_present": draft.is_file(),
        "draft_sha256": sha256_file(draft) if draft.is_file() else None,
        "raw_events_sha256": sha256_file(stdout_path),
        "stderr_sha256": sha256_file(stderr_path),
        "usage_sha256": sha256_file(usage_path) if usage_path.is_file() else None,
        "mutations": {
            "request_copy": len(request_changes),
            "canonical_request": len(canonical_changes),
            "source_repository": len(repository_changes),
            "trust_boundary": len(trust_changes),
        },
        "mutation_paths": {
            "request_copy": request_changes,
            "canonical_request": canonical_changes,
            "source_repository": repository_changes,
            "trust_boundary": trust_changes,
        },
    }
    (run_root / "execution.json").write_text(
        json.dumps(record, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    shutil.rmtree(workspace)
    return record


def execute(args: argparse.Namespace) -> None:
    repo = Path(__file__).resolve().parents[2]
    execution_root = Path(args.execution_root).resolve()
    copilot = Path(args.copilot).resolve()
    distill = Path(args.distill).resolve()
    if (execution_root / "execution.json").exists() or (execution_root / "canonical").exists():
        raise RuntimeError("execution root already contains a study execution")
    execution_root.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(execution_root, 0o700)
    validate_frozen_inputs(repo, copilot)
    prompt_path = repo / "research/handoff-semantic-draft-compiler/prompt.txt"
    prompt = prompt_path.read_text(encoding="utf-8")
    canonical_records = prepare_requests(repo, execution_root, distill)
    repository_before = file_manifest(repo, exclude_git=True)

    records = []
    for position, (run_name, condition_name, ordinal) in enumerate(RUN_ORDER, start=1):
        record = execute_run(
            repo,
            execution_root,
            copilot,
            prompt,
            canonical_records[condition_name],
            run_name,
            condition_name,
            ordinal,
            position,
            repository_before,
        )
        records.append(record)
        protected_mutations = (
            record["mutations"]["canonical_request"]
            + record["mutations"]["source_repository"]
        )
        if protected_mutations:
            break

    execution = {
        "schema_name": "handoff-semantic-draft-execution",
        "schema_version": 1,
        "protocol_sha256": PROTOCOL_SHA256,
        "prompt_sha256": PROMPT_SHA256,
        "semantic_draft_schema_sha256": SCHEMA_SHA256,
        "repository_baseline": BASELINE,
        "run_order": [name for name, _, _ in RUN_ORDER],
        "planned_invocations": len(RUN_ORDER),
        "completed_invocations": len(records),
        "canonical_requests": canonical_records,
        "runs": records,
    }
    (execution_root / "execution.json").write_text(
        json.dumps(execution, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    if len(records) != len(RUN_ORDER):
        raise RuntimeError("study stopped after a protected mutation")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--execution-root", required=True)
    parser.add_argument("--copilot", required=True)
    parser.add_argument("--distill", required=True)
    return parser.parse_args()


if __name__ == "__main__":
    execute(parse_args())
