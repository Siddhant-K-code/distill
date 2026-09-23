#!/usr/bin/env python3
"""Trusted offline MLX adapter for the local context-control study."""

from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.metadata
import json
import math
import os
import platform
import re
import stat
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SCHEMA = "context-build-artifact/local-context-control/v2"
ADAPTER_SCHEMA = SCHEMA + "/mlx-adapter"
RUNTIME_SCHEMA = SCHEMA + "/runtime-manifest"
MODEL_ARTIFACT_SHA256 = "057a37f4ebc76420f8ab2edb17bc8442e050c8d13f7334f829356e2f9cab6802"
TARGET_MODEL_ID = "Qwen/Qwen3-4B"
TARGET_MODEL_REVISION = "1cfa9a7208912126459214e8b04321603b3df60c"
MODEL_MANIFEST_SCHEMA = SCHEMA + "/model-manifest"
MODEL_TOTAL_BYTES = 2_274_515_269
RUNTIME_PACKAGE_CLOSURE_SHA256 = "119a039f08656f87e0a847a2ec9d9ba0633c12418fd5e08c469e67a262e46997"
RUNTIME_REQUIREMENTS_INPUT_SHA256 = "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b"
RUNTIME_REQUIREMENTS_SHA256 = "779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1"
RUNTIME_WHEEL_VERIFICATION_SHA256 = "b628d0b295cb4ce1cac3c51489930281581b9de4b7ad8e168d2e154a6cc98d3a"
RUNTIME_VENV_MANIFEST_SHA256 = "0941d4489820fffe045875d9e192378af135c8d0ca7d1d6c4f4b240d74b0f222"
BASE_PYTHON_BINARY_SHA256 = "4e1dfb03f82c5f7f253bbc3c04a79bb9f09a5cd0528829c32d6984ef309ebb2f"
BASE_PYTHON_MANIFEST_SHA256 = "295be0913c3d0390eccd7ffbc456874677a8e7b417de8bd90cae13102cc5a9d5"
MODEL_FILES = {
    "README.md": (81, "1a127c49a60fb70816260c037e7b54b3b5831e05ba4e4427dd016bf579247943"),
    "chat_template.jinja": (4168, "a55ee1b1660128b7098723e0abcd92caa0788061051c62d51cbe87d9cf1974d8"),
    "config.json": (1021, "cd08d43fc6f59e643b58b114d308a29bbd4ca147d30df6f419fcbe3115dcc2dd"),
    "generation_config.json": (239, "2325da0f15bb848e018c5ae071b7943332e9f871d6b60e2ed22ca97d4cb993d2"),
    "model.safetensors": (2_263_022_417, "365a6fff57490ca4e89e354c3295b7f2f60ac3fcb0a8a3fa06e21b80bb4d19ca"),
    "model.safetensors.index.json": (63_964, "388d811b8b7c2608dd04cce1bcb04a8bf715d19b42790894e6d3427ff429a777"),
    "tokenizer.json": (11_422_650, "be75606093db2094d7cd20f3c2f385c212750648bd6ea4fb2bf507a6a4c55506"),
    "tokenizer_config.json": (729, "ee8f6d44bf2353e6d3686c3adaf70e1ccfe9e6ed6822d0ab2f28cafdd7754792"),
}
EXPECTED_DISTRIBUTIONS = {
    "anyio": "4.9.0",
    "certifi": "2025.7.9",
    "click": "8.1.8",
    "filelock": "3.32.4",
    "fsspec": "2026.7.0",
    "h11": "0.16.0",
    "hf-xet": "1.6.0",
    "httpcore": "1.0.9",
    "httpx": "0.28.1",
    "huggingface-hub": "1.13.0",
    "idna": "3.15",
    "jinja2": "3.1.6",
    "markdown-it-py": "3.0.0",
    "markupsafe": "3.0.2",
    "mdurl": "0.1.2",
    "mlx": "0.32.2",
    "mlx-lm": "0.31.3",
    "mlx-metal": "0.32.2",
    "numpy": "2.2.6",
    "packaging": "26.3",
    "protobuf": "6.33.5",
    "pygments": "2.20.0",
    "pyyaml": "6.0.2",
    "regex": "2026.7.19",
    "rich": "14.0.0",
    "safetensors": "0.8.0",
    "sentencepiece": "0.2.2",
    "shellingham": "1.5.4",
    "sniffio": "1.3.1",
    "tokenizers": "0.23.1",
    "tqdm": "4.70.0",
    "transformers": "5.16.1",
    "typer": "0.16.0",
    "typing-extensions": "4.14.1",
}
FORBIDDEN_NAMES = {"sitecustomize.py", "usercustomize.py"}
FORBIDDEN_SUFFIXES = {".pth", ".pyc", ".pyo"}


class AdapterError(RuntimeError):
    pass


class FramingError(AdapterError):
    def __init__(self, status: str, message: str = "") -> None:
        super().__init__(message or status)
        self.status = status


class DuplicateKey(FramingError):
    def __init__(self, key: str) -> None:
        super().__init__("duplicate_key", key)


def canonical_number(value: float) -> str:
    if not math.isfinite(value):
        raise ValueError("nonfinite number")
    if value == 0:
        return "0"
    negative = value < 0
    raw = repr(abs(value)).lower()
    if "e" in raw:
        mantissa, exponent_raw = raw.split("e", 1)
        exponent = int(exponent_raw)
        digits = mantissa.replace(".", "")
        decimal_position = (mantissa.find(".") if "." in mantissa else len(mantissa)) + exponent
        if 1e-6 <= abs(value) < 1e21:
            if decimal_position <= 0:
                rendered = "0." + ("0" * -decimal_position) + digits
            elif decimal_position >= len(digits):
                rendered = digits + ("0" * (decimal_position - len(digits)))
            else:
                rendered = digits[:decimal_position] + "." + digits[decimal_position:]
        else:
            coefficient = digits[0]
            if len(digits) > 1:
                coefficient += "." + digits[1:]
            normalized_exponent = decimal_position - 1
            sign = "+" if normalized_exponent >= 0 else ""
            rendered = f"{coefficient}e{sign}{normalized_exponent}"
    else:
        rendered = raw[:-2] if raw.endswith(".0") else raw
    return ("-" if negative else "") + rendered


def canonical_json(value: Any) -> bytes:
    def encode(item: Any) -> str:
        if item is None:
            return "null"
        if item is True:
            return "true"
        if item is False:
            return "false"
        if isinstance(item, int):
            if abs(item) > 9_007_199_254_740_991:
                raise ValueError("integer exceeds exact JSON range")
            return str(item)
        if isinstance(item, float):
            return canonical_number(item)
        if isinstance(item, str):
            return json.dumps(item, ensure_ascii=False, separators=(",", ":"))
        if isinstance(item, list):
            return "[" + ",".join(encode(element) for element in item) + "]"
        if isinstance(item, dict):
            if any(not isinstance(key, str) for key in item):
                raise TypeError("canonical JSON object keys must be strings")
            return "{" + ",".join(
                encode(key) + ":" + encode(item[key])
                for key in sorted(item, key=lambda value: value.encode("utf-16-be"))
            ) + "}"
        raise TypeError(f"unsupported canonical JSON value: {type(item).__name__}")

    return encode(value).encode("utf-8")


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def domain_digest(domain: str, value: Any) -> str:
    prefix = f"context-build-artifact/local-context-control/{domain}/v2\n".encode()
    return sha256(prefix + canonical_json(value))


def no_duplicate_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise DuplicateKey(key)
        value[key] = item
    return value


def strict_integer(raw: str) -> int:
    value = int(raw)
    if abs(value) > 9_007_199_254_740_991:
        raise FramingError("number_out_of_range")
    return value


def strict_float(raw: str) -> float:
    exponent = re.search(r"[eE]([+-]?\d+)$", raw)
    if exponent is not None and abs(int(exponent.group(1))) > 308:
        raise FramingError("number_out_of_range")
    value = float(raw)
    if not math.isfinite(value):
        raise FramingError("number_out_of_range")
    return value


def reject_constant(_: str) -> float:
    raise FramingError("nonfinite_number")


def strict_json_payload(data: bytes, *, require_object: bool) -> Any:
    if not data:
        raise FramingError("invalid_json")
    if data[:1] in {b" ", b"\t", b"\n", b"\r"}:
        raise FramingError("leading_whitespace")
    if b"\r" in data:
        raise FramingError("carriage_return")
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise FramingError("invalid_json", "JSON is not UTF-8") from exc
    decoder = json.JSONDecoder(
        object_pairs_hook=no_duplicate_object,
        parse_constant=reject_constant,
        parse_float=strict_float,
        parse_int=strict_integer,
    )
    try:
        value, end = decoder.raw_decode(text)
    except FramingError:
        raise
    except json.JSONDecodeError as exc:
        raise FramingError("invalid_json") from exc
    if end != len(text):
        if text[end:].isspace():
            raise FramingError("non_canonical")
        raise FramingError("trailing_bytes")
    if require_object and not isinstance(value, dict):
        raise FramingError("non_object")
    if canonical_json(value) != data:
        raise FramingError("non_canonical")
    return value


def strict_envelope_payload(data: bytes) -> bytes:
    if not data.endswith(b"\n"):
        raise FramingError("missing_terminal_lf")
    if data.endswith(b"\n\n"):
        raise FramingError("multiple_terminal_lf")
    if b"\r" in data:
        raise FramingError("carriage_return")
    return data[:-1]


def strict_json_file(data: bytes, *, require_object: bool = True) -> Any:
    return strict_json_payload(strict_envelope_payload(data), require_object=require_object)


def strict_json_frame(data: bytes, *, require_object: bool = True) -> Any:
    return strict_json_payload(strict_envelope_payload(data), require_object=require_object)


def write_frame(value: Any) -> None:
    sys.stdout.buffer.write(canonical_json(value) + b"\n")
    sys.stdout.buffer.flush()


def safe_directory(raw: str) -> Path:
    path = Path(raw)
    if not path.is_absolute() or path.is_symlink() or not path.is_dir():
        raise AdapterError("trusted directory must be absolute, real, and present")
    if path.resolve(strict=True) != path:
        raise AdapterError("trusted directory path contains a symlink")
    if path.stat().st_mode & 0o022:
        raise AdapterError("trusted directory is group/world writable")
    return path


def safe_file(raw: str) -> Path:
    path = Path(raw)
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise AdapterError("trusted file must be absolute, regular, and present")
    if path.resolve(strict=True) != path:
        raise AdapterError("trusted file path contains a symlink")
    if path.stat().st_mode & 0o022:
        raise AdapterError("trusted file is group/world writable")
    return path


def file_identity(path: Path) -> tuple[int, str]:
    before = path.lstat()
    if not stat.S_ISREG(before.st_mode) or path.is_symlink():
        raise AdapterError("identity input is not a regular file")
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    after = path.lstat()
    before_key = (before.st_dev, before.st_ino, before.st_mode, before.st_size, before.st_mtime_ns)
    after_key = (after.st_dev, after.st_ino, after.st_mode, after.st_size, after.st_mtime_ns)
    if before_key != after_key:
        raise AdapterError("identity input changed while hashing")
    return before.st_size, digest.hexdigest()


def git(repository: Path, args: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        [
            "/usr/bin/git",
            "--no-replace-objects",
            "-c",
            "core.fsmonitor=false",
            "-c",
            "core.hooksPath=/dev/null",
            "-c",
            "core.attributesFile=/dev/null",
            "-C",
            str(repository),
            *args,
        ],
        env={
            "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
            "HOME": "/dev/null",
            "LC_ALL": "C",
            "LANG": "C",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_NO_LAZY_FETCH": "1",
            "GIT_NO_REPLACE_OBJECTS": "1",
        },
        capture_output=True,
        check=False,
    )


def verify_self(
    repository: Path,
    expected_commit: str,
    expected_sha256: str,
    expected_tree: str | None = None,
) -> None:
    if re.fullmatch(r"[0-9a-f]{40}", expected_commit) is None:
        raise AdapterError("expected commit must be a full lowercase Git object ID")
    script = safe_file(str(Path(__file__)))
    relative = script.relative_to(repository).as_posix()
    raw = script.read_bytes()
    if sha256(raw) != expected_sha256:
        raise AdapterError("adapter source digest mismatch")
    head = git(repository, ["rev-parse", "--verify", "HEAD^{commit}"])
    if head.returncode != 0 or head.stdout.strip().decode() != expected_commit:
        raise AdapterError("repository HEAD mismatch")
    if expected_tree is not None:
        tree = git(repository, ["rev-parse", "--verify", "HEAD^{tree}"])
        if tree.returncode != 0 or tree.stdout.strip().decode() != expected_tree:
            raise AdapterError("repository tree mismatch")
    committed = git(repository, ["show", f"{expected_commit}:{relative}"])
    if committed.returncode != 0 or committed.stdout != raw:
        raise AdapterError("adapter source does not match the authenticated commit")
    dirty = git(repository, ["status", "--porcelain=v1", "--untracked-files=no"])
    if dirty.returncode != 0 or dirty.stdout:
        raise AdapterError("repository has tracked modifications")


def normalized_name(name: str) -> str:
    return re.sub(r"[-_.]+", "-", name).lower()


def runtime_site_root() -> Path:
    if platform.python_implementation() != "CPython" or platform.python_version() != "3.13.15":
        raise AdapterError("runtime must be CPython 3.13.15")
    executable = Path(sys.executable)
    if not executable.is_absolute():
        raise AdapterError("runtime interpreter path is not absolute")
    root = executable.parent.parent
    site_root = root / "lib" / "python3.13" / "site-packages"
    safe_directory(str(root))
    return safe_directory(str(site_root))


def verify_tree_manifest(
    root: Path,
    manifest_path: Path,
    expected_digest: str,
    *,
    expected_files: int,
    expected_directories: int,
    expected_symlinks: int,
) -> None:
    raw = manifest_path.read_bytes()
    if sha256(raw) != expected_digest:
        raise AdapterError("private tree manifest digest mismatch")
    expected: dict[str, tuple[str, ...]] = {}
    for line in raw.decode("utf-8").splitlines():
        fields = line.split("\t")
        if len(fields) < 3 or fields[0] not in {"D", "F", "L"}:
            raise AdapterError("private tree manifest is malformed")
        kind, relative = fields[0], fields[1]
        if relative in expected or relative.startswith("/") or ".." in Path(relative).parts:
            raise AdapterError("private tree manifest path is unsafe")
        expected[relative] = tuple(fields)

    actual: set[str] = set()
    file_count = directory_count = symlink_count = 0
    for current, directories, filenames in os.walk(root, followlinks=False):
        current_path = Path(current)
        directories.sort()
        filenames.sort()
        retained: list[str] = []
        for name in directories:
            candidate = current_path / name
            relative = candidate.relative_to(root).as_posix()
            if candidate.is_symlink():
                fields = expected.get(relative)
                if fields != ("L", relative, oct(candidate.stat().st_mode & 0o777)[2:], os.readlink(candidate)):
                    # The private producer records effective target permissions for links.
                    if fields is None or len(fields) != 4 or fields[0] != "L" or fields[3] != os.readlink(candidate):
                        raise AdapterError("tree symlink identity mismatch")
                symlink_count += 1
            else:
                fields = expected.get(relative)
                mode = oct(candidate.stat().st_mode & 0o777)[2:]
                if fields != ("D", relative, mode):
                    raise AdapterError("tree directory identity mismatch")
                directory_count += 1
                retained.append(name)
            actual.add(relative)
        directories[:] = retained
        for name in filenames:
            candidate = current_path / name
            relative = candidate.relative_to(root).as_posix()
            fields = expected.get(relative)
            if candidate.is_symlink():
                if fields is None or len(fields) != 4 or fields[0] != "L" or fields[3] != os.readlink(candidate):
                    raise AdapterError("tree symlink identity mismatch")
                symlink_count += 1
            else:
                size, digest = file_identity(candidate)
                mode = oct(candidate.stat().st_mode & 0o777)[2:]
                if fields != ("F", relative, mode, str(size), digest):
                    raise AdapterError("tree file identity mismatch")
                file_count += 1
            actual.add(relative)
    if set(expected) != actual:
        raise AdapterError("tree manifest inventory mismatch")
    if (file_count, directory_count, symlink_count) != (
        expected_files,
        expected_directories,
        expected_symlinks,
    ):
        raise AdapterError("tree manifest count mismatch")


def runtime_manifest(args: argparse.Namespace) -> dict[str, Any]:
    site_root = runtime_site_root()
    venv_root = safe_directory(str(Path(sys.executable).parent.parent))
    base_python = Path(sys.executable).resolve(strict=True)
    base_root = safe_directory(str(base_python.parents[1]))
    runtime_tree_manifest = safe_file(args.runtime_tree_manifest)
    base_tree_manifest = safe_file(args.base_tree_manifest)
    wheel_verification = safe_file(args.wheel_verification)
    if sha256(wheel_verification.read_bytes()) != RUNTIME_WHEEL_VERIFICATION_SHA256:
        raise AdapterError("wheel verification record digest mismatch")
    verify_tree_manifest(
        venv_root,
        runtime_tree_manifest,
        RUNTIME_VENV_MANIFEST_SHA256,
        expected_files=5532,
        expected_directories=831,
        expected_symlinks=3,
    )
    verify_tree_manifest(
        base_root,
        base_tree_manifest,
        BASE_PYTHON_MANIFEST_SHA256,
        expected_files=1942,
        expected_directories=191,
        expected_symlinks=8,
    )
    if file_identity(base_python)[1] != BASE_PYTHON_BINARY_SHA256:
        raise AdapterError("base Python binary identity mismatch")
    distributions: list[dict[str, str]] = []
    discovered: dict[str, list[importlib.metadata.Distribution]] = {}
    for distribution in importlib.metadata.distributions(path=[str(site_root)]):
        raw_name = distribution.metadata.get("Name")
        if not isinstance(raw_name, str):
            raise AdapterError("runtime distribution has no name")
        discovered.setdefault(normalized_name(raw_name), []).append(distribution)
    if set(discovered) != set(EXPECTED_DISTRIBUTIONS):
        raise AdapterError("runtime distribution closure mismatch")
    for name, expected_version in sorted(EXPECTED_DISTRIBUTIONS.items()):
        matches = discovered[name]
        if len(matches) != 1 or matches[0].version != expected_version:
            raise AdapterError(f"runtime distribution mismatch: {name}")
        distributions.append({"name": name, "version": expected_version})

    tree = hashlib.sha256()
    site_file_count = 0
    bytecode = 0
    startup_hooks = 0
    symlinks = 0
    for current, directories, filenames in os.walk(site_root, followlinks=False):
        current_path = Path(current)
        directories.sort()
        filenames.sort()
        for name in directories:
            candidate = current_path / name
            if candidate.is_symlink():
                symlinks += 1
            if name == "__pycache__":
                bytecode += 1
        for name in filenames:
            candidate = current_path / name
            if candidate.is_symlink():
                symlinks += 1
                continue
            if name in FORBIDDEN_NAMES:
                startup_hooks += 1
            if candidate.suffix.lower() in FORBIDDEN_SUFFIXES or "__pycache__" in candidate.parts:
                bytecode += 1
            size, digest = file_identity(candidate)
            relative = candidate.relative_to(site_root).as_posix()
            record = {"path": relative, "sha256": digest, "size_bytes": size}
            tree.update(canonical_json(record))
            tree.update(b"\n")
            site_file_count += 1
    manifest = {
        "schema_version": RUNTIME_SCHEMA,
        "python_version": platform.python_version(),
        "distribution_count": len(distributions),
        "distributions": distributions,
        "file_count": 5532,
        "total_bytes": 335_118_336,
        "full_tree_sha256": RUNTIME_VENV_MANIFEST_SHA256,
        "venv_directory_count": 831,
        "venv_symlink_count": 3,
        "base_file_count": 1942,
        "base_directory_count": 191,
        "base_symlink_count": 8,
        "base_tree_sha256": BASE_PYTHON_MANIFEST_SHA256,
        "base_python_sha256": BASE_PYTHON_BINARY_SHA256,
        "package_closure_sha256": RUNTIME_PACKAGE_CLOSURE_SHA256,
        "requirements_input_sha256": RUNTIME_REQUIREMENTS_INPUT_SHA256,
        "requirements_lock_sha256": RUNTIME_REQUIREMENTS_SHA256,
        "wheel_verification_sha256": RUNTIME_WHEEL_VERIFICATION_SHA256,
        "site_packages_file_count": site_file_count,
        "site_packages_tree_sha256": tree.hexdigest(),
        "manifest_sha256": "",
        "bytecode_files": bytecode,
        "startup_hooks": startup_hooks,
        "symlinks": symlinks,
    }
    manifest["manifest_sha256"] = domain_digest("runtime-manifest", manifest)
    return manifest


def verify_model(model_path: Path, manifest_path: Path) -> dict[str, Any]:
    expected = strict_json_file(manifest_path.read_bytes())
    exact_object(
        expected,
        {
            "schema_version", "official_model_id", "official_revision", "format",
            "quantization_mode", "quantization_bits", "quantization_group_size",
            "converter_package", "converter_version", "converter_revision",
            "conversion_manifest_sha256", "file_manifest_sha256", "files",
            "file_count", "total_bytes", "artifact_sha256", "manifest_sha256",
        },
        "model manifest",
    )
    if not isinstance(expected, dict) or expected.get("schema_version") != MODEL_MANIFEST_SCHEMA:
        raise AdapterError("model manifest schema mismatch")
    entries = sorted(model_path.iterdir(), key=lambda item: item.name)
    if len(entries) != len(MODEL_FILES) or any(item.name not in MODEL_FILES for item in entries):
        raise AdapterError("model file allowlist mismatch")
    total = 0
    actual_files = []
    for item in entries:
        size, digest = file_identity(item)
        if (size, digest) != MODEL_FILES[item.name]:
            raise AdapterError(f"model file identity mismatch: {item.name}")
        actual_files.append({"path": item.name, "size_bytes": size, "sha256": digest})
        total += size
    if total != MODEL_TOTAL_BYTES or expected.get("files") != actual_files:
        raise AdapterError("model manifest differs from local files")
    if expected.get("artifact_sha256") != MODEL_ARTIFACT_SHA256:
        raise AdapterError("model artifact contract mismatch")
    projection = dict(expected)
    manifest_digest = projection.pop("manifest_sha256", None)
    projection["manifest_sha256"] = ""
    if manifest_digest != domain_digest("model-manifest", projection):
        raise AdapterError("model manifest digest mismatch")
    return expected


def parse_projection(raw: bytes, allowed: list[str]) -> dict[str, Any]:
    status = "invalid_json"
    answer: str | None = None
    try:
        text = raw.decode("utf-8")
        decoder = json.JSONDecoder(object_pairs_hook=no_duplicate_object)
        value, end = decoder.raw_decode(text)
        if end != len(text):
            status = "trailing_text"
        elif not isinstance(value, dict):
            status = "invalid_json"
        elif set(value) != {"answer"} or not isinstance(value["answer"], str):
            status = "schema_error"
        elif value["answer"] not in allowed:
            status = "invalid_enum"
        else:
            status = "valid"
            answer = value["answer"]
    except DuplicateKey:
        status = "duplicate_key"
    except (UnicodeDecodeError, json.JSONDecodeError):
        status = "invalid_json"
    return {
        "answer": answer,
        "confidence": None,
        "confidence_basis": "unavailable",
        "parse_status": status,
    }


def exact_object(value: Any, keys: set[str], context: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != keys:
        raise AdapterError(f"{context} field allowlist mismatch")
    return value


def generation_request(value: Any) -> dict[str, Any]:
    request = exact_object(
        value,
        {"schema_version", "observation_id", "system_prompt", "context", "questions", "generation"},
        "adapter request",
    )
    if request["schema_version"] != SCHEMA + "/adapter-request":
        raise AdapterError("adapter request schema mismatch")
    if not isinstance(request["observation_id"], str) or not request["observation_id"]:
        raise AdapterError("observation identity missing")
    if not isinstance(request["system_prompt"], str) or not isinstance(request["context"], str):
        raise AdapterError("prompt inputs must be strings")
    lowered = request["context"].lower()
    if "agenttrace" in lowered or "agent-trace" in lowered or "api.typesafe.ai" in lowered:
        raise AdapterError("forbidden held-out or provider content")
    generation = exact_object(
        request["generation"],
        {
            "sampling",
            "temperature",
            "seed",
            "seed_guarantee",
            "max_output_tokens",
            "chat_template",
            "chat_template_options",
            "stop_rules",
            "attempts_per_scheduled_observation",
        },
        "generation",
    )
    if (
        generation["sampling"] != "greedy_mlx_make_sampler_temp_0"
        or generation["temperature"] != 0
        or generation["seed"] != 20260921
        or generation["max_output_tokens"] != 256
        or generation["attempts_per_scheduled_observation"] != 1
    ):
        raise AdapterError("generation contract drift")
    if not isinstance(request["questions"], list) or not request["questions"]:
        raise AdapterError("questions are missing")
    for question in request["questions"]:
        exact_object(question, {"id", "text", "allowed"}, "question")
        if (
            not isinstance(question["id"], str)
            or not isinstance(question["text"], str)
            or not isinstance(question["allowed"], list)
            or not question["allowed"]
            or any(not isinstance(item, str) for item in question["allowed"])
        ):
            raise AdapterError("question contract is invalid")
    return request


def memory_snapshot(mx: Any) -> dict[str, int]:
    return {
        "active_bytes": int(mx.get_active_memory()),
        "cache_bytes": int(mx.get_cache_memory()),
        "peak_bytes": int(mx.get_peak_memory()),
    }


def generate_question(
    mx: Any, stream_generate: Any, make_sampler: Any, model: Any, tokenizer: Any,
    request: dict[str, Any], question: dict[str, Any],
) -> dict[str, Any]:
    prompt_started = time.perf_counter_ns()
    user_text = (
        "EVIDENCE CONTEXT\n"
        + request["context"]
        + "\nQUESTION\n"
        + question["text"]
        + "\nALLOWED ANSWERS\n"
        + " | ".join(question["allowed"])
    )
    prompt_ids = tokenizer.apply_chat_template(
        [
            {"role": "system", "content": request["system_prompt"]},
            {"role": "user", "content": user_text},
        ],
        tokenize=True,
        add_generation_prompt=True,
        enable_thinking=False,
    )
    if hasattr(prompt_ids, "tolist"):
        prompt_ids = prompt_ids.tolist()
    prompt_ids = [int(token) for token in prompt_ids]
    mx.synchronize()
    tokenize_ended = time.perf_counter_ns()
    mx.random.seed(int(request["generation"]["seed"]))
    mx.reset_peak_memory()
    generation_started = time.perf_counter_ns()
    first_token_at: int | None = None
    generated_ids: list[int] = []
    text_parts: list[str] = []
    sampler = make_sampler(temp=0.0)
    for response in stream_generate(
        model,
        tokenizer,
        prompt_ids,
        max_tokens=int(request["generation"]["max_output_tokens"]),
        sampler=sampler,
    ):
        if first_token_at is None:
            mx.synchronize()
            first_token_at = time.perf_counter_ns()
        generated_ids.append(int(response.token))
        text_parts.append(response.text)
    mx.synchronize()
    generation_ended = time.perf_counter_ns()
    if first_token_at is None:
        first_token_at = generation_ended
    raw = "".join(text_parts).encode("utf-8")
    projection = parse_projection(raw, question["allowed"])
    return {
        "question_id": question["id"],
        "raw_response_base64": base64.b64encode(raw).decode("ascii"),
        "raw_response_sha256": sha256(raw),
        "prompt_token_ids": prompt_ids,
        "generated_token_ids": generated_ids,
        "prompt_tokens": len(prompt_ids),
        "generated_tokens": len(generated_ids),
        "projection": projection,
        "timing": {
            "tokenize_nanoseconds": tokenize_ended - prompt_started,
            "ttft_nanoseconds": first_token_at - generation_started,
            "decode_nanoseconds": generation_ended - first_token_at,
            "total_nanoseconds": generation_ended - prompt_started,
        },
        "memory": memory_snapshot(mx),
    }


def verify_run_bindings(
    args: argparse.Namespace,
    model_manifest: dict[str, Any],
    runtime: dict[str, Any],
) -> dict[str, Any]:
    package_raw = safe_file(args.package_manifest).read_bytes()
    package_manifest = strict_json_file(package_raw)
    exact_object(
        package_manifest,
        {
            "schema_version", "corpus_sha256", "protocol_sha256",
            "contexts_sha256", "schedule_sha256", "result_schema_sha256",
            "source_registry_sha256", "package_file_count", "provider_calls",
            "execution_authorized",
        },
        "package manifest",
    )
    authorization_raw = safe_file(args.authorization).read_bytes()
    authorization = strict_json_file(authorization_raw)
    exact_object(
        authorization,
        {
            "schema_version", "created_at", "acknowledgement",
            "implementation_commit", "implementation_tree", "main_ref",
            "build_package", "build_go_version", "build_vcs_revision",
            "build_vcs_modified", "tool_sha256", "adapter_sha256",
            "package_manifest_sha256", "schedule_sha256",
            "model_manifest_sha256", "model_artifact_sha256",
            "runtime_manifest_sha256", "runtime_full_tree_sha256",
            "adapter_check_request_sha256", "adapter_check_receipt_sha256",
            "adapter_check_directory_sha256", "adapter_check_expires_at",
            "framing_conformance_sha256", "protocol_sha256",
            "output_namespace_sha256", "authorization_directory_sha256",
            "attempt_nonce", "ledger_genesis_sha256", "observations",
            "network_allowed", "cloud_spend_authorized",
            "scaling_control_enabled", "authorization_sha256",
        },
        "execution authorization",
    )
    authorization_projection = dict(authorization)
    authorization_projection["authorization_sha256"] = ""
    if (
        authorization["schema_version"] != SCHEMA + "/execution-authorization"
        or authorization["authorization_sha256"]
        != domain_digest("execution-authorization", authorization_projection)
        or authorization["implementation_commit"] != args.expected_commit
        or authorization["adapter_sha256"] != args.expected_self_sha256
        or authorization["package_manifest_sha256"] != sha256(package_raw)
        or authorization["protocol_sha256"] != package_manifest["protocol_sha256"]
        or authorization["schedule_sha256"] != package_manifest["schedule_sha256"]
        or authorization["model_manifest_sha256"] != model_manifest["manifest_sha256"]
        or authorization["model_artifact_sha256"] != model_manifest["artifact_sha256"]
        or authorization["runtime_manifest_sha256"] != runtime["manifest_sha256"]
        or authorization["runtime_full_tree_sha256"] != runtime["full_tree_sha256"]
        or authorization["network_allowed"] is not False
        or authorization["cloud_spend_authorized"] is not False
        or authorization["scaling_control_enabled"] is not False
    ):
        raise AdapterError("execution authorization binding mismatch")

    check = strict_json_file(safe_file(args.adapter_check_receipt).read_bytes())
    check_keys = {
        "schema_version", "request_sha256", "started_at", "completed_at",
        "expires_at", "implementation_commit", "implementation_tree",
        "build_package", "build_go_version", "build_vcs_revision",
        "build_vcs_modified", "input_file_sha256", "package_manifest_sha256",
        "protocol_sha256", "schedule_sha256", "model_manifest_sha256",
        "model_artifact_sha256", "runtime_manifest_sha256",
        "runtime_full_tree_sha256", "runtime_interpreter_sha256",
        "adapter_sha256", "tool_sha256", "host_preflight_sha256",
        "conformance_request_sha256", "framing_conformance_sha256",
        "framing_conformance_vectors", "output_namespace_sha256",
        "adapter_check_directory_sha256", "model_loaded", "network_allowed",
        "receipt_sha256",
    }
    exact_object(check, check_keys, "adapter-check receipt")
    check_projection = dict(check)
    check_projection["receipt_sha256"] = ""
    if (
        check["schema_version"] != SCHEMA + "/adapter-check-receipt"
        or check["receipt_sha256"] != domain_digest("adapter-check-receipt", check_projection)
        or check["receipt_sha256"] != authorization["adapter_check_receipt_sha256"]
        or check["request_sha256"] != authorization["adapter_check_request_sha256"]
        or check["framing_conformance_sha256"]
        != authorization["framing_conformance_sha256"]
        or check["model_loaded"] is not False
        or check["network_allowed"] is not False
    ):
        raise AdapterError("adapter-check receipt binding mismatch")

    run_namespace = safe_directory(args.run_namespace)
    if sha256(str(run_namespace).encode()) != authorization["output_namespace_sha256"]:
        raise AdapterError("run namespace binding mismatch")
    output = Path(args.output_namespace)
    if not output.is_absolute() or output.exists():
        raise AdapterError("adapter output namespace must be an absent absolute path")
    for forbidden_parent in (
        repository := safe_directory(args.trusted_repo_root),
        safe_directory(args.model_path),
        Path(sys.executable).parent.parent,
    ):
        try:
            output.relative_to(forbidden_parent)
        except ValueError:
            pass
        else:
            raise AdapterError("adapter output namespace overlaps a trusted input")
    if repository != safe_directory(args.trusted_repo_root):
        raise AdapterError("repository identity changed")
    return {
        "package_manifest_sha256": sha256(package_raw),
        "authorization_sha256": authorization["authorization_sha256"],
        "adapter_check_receipt_sha256": check["receipt_sha256"],
        "output_namespace_sha256": authorization["output_namespace_sha256"],
        "schedule_sha256": authorization["schedule_sha256"],
        "attempt_nonce": authorization["attempt_nonce"],
    }


def verify_run_manifest(path: str, expected_sha256: str, bindings: dict[str, Any]) -> None:
    raw = safe_file(path).read_bytes()
    if sha256(raw) != expected_sha256:
        raise AdapterError("run manifest frame binding mismatch")
    manifest = strict_json_file(raw)
    exact_object(
        manifest,
        {
            "schema_version", "started_at", "implementation_commit",
            "authorization_sha256", "attempt_nonce",
            "attempt_ledger_entry_sha256", "package_manifest_sha256",
            "schedule_sha256", "observations_expected",
            "attempts_per_observation", "model_id", "model_revision",
            "scaling_control_enabled", "network_allowed",
        },
        "run manifest",
    )
    if (
        manifest["schema_version"] != SCHEMA + "/run-manifest"
        or manifest["implementation_commit"] != bindings["implementation_commit"]
        or manifest["authorization_sha256"] != bindings["authorization_sha256"]
        or manifest["package_manifest_sha256"] != bindings["package_manifest_sha256"]
        or manifest["schedule_sha256"] != bindings["schedule_sha256"]
        or manifest["attempt_nonce"] != bindings["attempt_nonce"]
        or manifest["observations_expected"] != 332
        or manifest["model_id"] != TARGET_MODEL_ID
        or manifest["model_revision"] != TARGET_MODEL_REVISION
        or manifest["network_allowed"] is not False
        or manifest["scaling_control_enabled"] is not False
        or manifest["attempts_per_observation"] != 1
    ):
        raise AdapterError("run manifest binding mismatch")


def serve(args: argparse.Namespace) -> None:
    repository = safe_directory(args.trusted_repo_root)
    verify_self(repository, args.expected_commit, args.expected_self_sha256)
    model_path = safe_directory(args.model_path)
    model_manifest_path = safe_file(args.model_manifest)
    runtime_manifest_path = safe_file(args.runtime_manifest)
    expected_runtime = strict_json_file(runtime_manifest_path.read_bytes())
    actual_runtime = runtime_manifest(args)
    if actual_runtime != expected_runtime:
        raise AdapterError("runtime tree differs from the frozen runtime manifest")
    model_manifest = verify_model(model_path, model_manifest_path)
    bindings = verify_run_bindings(args, model_manifest, expected_runtime)
    preload = {
        "schema_version": ADAPTER_SCHEMA + "/preload-ready",
        "adapter_sha256": args.expected_self_sha256,
        "implementation_commit": args.expected_commit,
        "package_manifest_sha256": bindings["package_manifest_sha256"],
        "authorization_sha256": bindings["authorization_sha256"],
        "adapter_check_receipt_sha256": bindings["adapter_check_receipt_sha256"],
        "model_manifest_sha256": model_manifest["manifest_sha256"],
        "runtime_manifest_sha256": expected_runtime["manifest_sha256"],
        "output_namespace_sha256": bindings["output_namespace_sha256"],
        "model_loaded": False,
    }
    bindings["implementation_commit"] = args.expected_commit
    write_frame(preload)
    load_frame = sys.stdin.buffer.readline()
    load_request = strict_json_frame(load_frame)
    exact_object(load_request, {"command", "run_manifest_sha256"}, "load request")
    if load_request["command"] != "load" or not isinstance(load_request["run_manifest_sha256"], str):
        raise AdapterError("first adapter command must be load")
    verify_run_manifest(args.run_manifest, load_request["run_manifest_sha256"], bindings)

    load_started = time.perf_counter_ns()
    sys.path.insert(0, str(runtime_site_root()))
    import mlx.core as mx
    from mlx_lm import load, stream_generate
    from mlx_lm.sample_utils import make_sampler

    model, tokenizer = load(str(model_path), lazy=False, return_config=False)
    mx.synchronize()
    load_ended = time.perf_counter_ns()
    ready = {
        "schema_version": ADAPTER_SCHEMA + "/ready",
        "adapter_sha256": args.expected_self_sha256,
        "implementation_commit": args.expected_commit,
        "model_manifest_sha256": model_manifest["manifest_sha256"],
        "runtime_manifest_sha256": expected_runtime["manifest_sha256"],
        "model_loaded": True,
        "load_nanoseconds": load_ended - load_started,
    }
    write_frame(ready)

    for line in sys.stdin.buffer:
        value = strict_json_frame(line)
        if value == {"command": "close"}:
            return
        request = generation_request(value)
        started = time.perf_counter_ns()
        questions = [
            generate_question(mx, stream_generate, make_sampler, model, tokenizer, request, question)
            for question in request["questions"]
        ]
        response = {
            "schema_version": ADAPTER_SCHEMA + "/response",
            "observation_id": request["observation_id"],
            "questions": questions,
        }
        if time.perf_counter_ns() < started:
            raise AdapterError("monotonic clock moved backwards")
        write_frame(response)


def framing_conformance(request: Any) -> dict[str, Any]:
    request = exact_object(request, {"schema_version", "vectors"}, "framing request")
    if request["schema_version"] != SCHEMA + "/framing-conformance/request":
        raise AdapterError("framing request schema mismatch")
    if not isinstance(request["vectors"], list) or not request["vectors"]:
        raise AdapterError("framing vectors are missing")
    results = []
    for vector in request["vectors"]:
        vector = exact_object(
            vector,
            {"name", "artifact_class", "data_base64", "require_object", "expected_status"},
            "framing vector",
        )
        if (
            not isinstance(vector["name"], str)
            or vector["artifact_class"] not in {"file", "frame"}
            or not isinstance(vector["data_base64"], str)
            or not isinstance(vector["require_object"], bool)
            or not isinstance(vector["expected_status"], str)
        ):
            raise AdapterError("framing vector value mismatch")
        try:
            raw = base64.b64decode(vector["data_base64"], validate=True)
            if vector["artifact_class"] == "file":
                strict_json_file(raw, require_object=vector["require_object"])
            else:
                strict_json_frame(raw, require_object=vector["require_object"])
            status = "valid"
        except FramingError as exc:
            status = exc.status
        if status != vector["expected_status"]:
            raise AdapterError(
                f"framing vector {vector['name']}: got {status}, "
                f"want {vector['expected_status']}"
            )
        results.append({"name": vector["name"], "status": status})
    response = {
        "schema_version": SCHEMA + "/framing-conformance/response",
        "request_sha256": domain_digest("framing-conformance-request", request),
        "results": results,
        "conformance_sha256": "",
        "implementation": "python",
        "model_runtime_imported": False,
    }
    projection = dict(response)
    projection["implementation"] = ""
    response["conformance_sha256"] = domain_digest("framing-conformance", projection)
    return response


def exact_file_hash(path: Path, expected: str, label: str) -> None:
    if file_identity(path)[1] != expected:
        raise AdapterError(f"{label} input hash mismatch")


def adapter_check(args: argparse.Namespace) -> dict[str, Any]:
    if any(name == "mlx" or name.startswith("mlx.") or name == "mlx_lm" or name.startswith("mlx_lm.") for name in sys.modules):
        raise AdapterError("adapter-check process has imported model runtime modules")
    request_path = safe_file(args.request)
    request = strict_json_file(request_path.read_bytes())
    request_keys = {
        "schema_version", "started_at", "expires_at", "implementation_commit",
        "implementation_tree", "main_ref", "build_package", "build_go_version",
        "build_vcs_revision", "build_vcs_modified", "tool_path", "adapter_path",
        "repository_root", "package_manifest_path", "model_directory",
        "model_manifest_path", "runtime_python", "runtime_manifest_path",
        "runtime_tree_manifest_path", "base_tree_manifest_path",
        "wheel_verification_path", "host_preflight_path",
        "conformance_request_path", "output_namespace", "adapter_check_directory",
        "input_file_sha256", "package_manifest_sha256", "protocol_sha256",
        "schedule_sha256", "model_manifest_sha256", "model_artifact_sha256",
        "runtime_manifest_sha256", "runtime_full_tree_sha256",
        "host_preflight_sha256", "conformance_request_sha256",
        "output_namespace_sha256", "adapter_check_directory_sha256",
        "model_loaded_expected", "network_allowed", "request_sha256",
    }
    exact_object(request, request_keys, "adapter-check request")
    if request["schema_version"] != SCHEMA + "/adapter-check-request":
        raise AdapterError("adapter-check request schema mismatch")
    projection = dict(request)
    projection["request_sha256"] = ""
    if request["request_sha256"] != domain_digest("adapter-check-request", projection):
        raise AdapterError("adapter-check request digest mismatch")
    if (
        request["main_ref"] != "origin/main"
        or request["build_vcs_revision"] != request["implementation_commit"]
        or request["build_vcs_modified"] is not False
        or request["model_loaded_expected"] is not False
        or request["network_allowed"] is not False
    ):
        raise AdapterError("adapter-check request policy mismatch")
    started = datetime.fromisoformat(request["started_at"].replace("Z", "+00:00"))
    expires = datetime.fromisoformat(request["expires_at"].replace("Z", "+00:00"))
    if started.tzinfo is None or expires.tzinfo is None or expires <= started:
        raise AdapterError("adapter-check time bounds are invalid")

    repository = safe_directory(request["repository_root"])
    adapter_path = safe_file(request["adapter_path"])
    if adapter_path != Path(__file__):
        raise AdapterError("adapter path does not identify this source")
    verify_self(
        repository,
        request["implementation_commit"],
        request["input_file_sha256"]["adapter"],
        request["implementation_tree"],
    )
    check_directory = safe_directory(request["adapter_check_directory"])
    if request_path.parent != check_directory:
        raise AdapterError("adapter-check request is outside its bound directory")
    if sha256(str(check_directory).encode()) != request["adapter_check_directory_sha256"]:
        raise AdapterError("adapter-check directory binding mismatch")

    input_paths = {
        "adapter": adapter_path,
        "tool": safe_file(request["tool_path"]),
        "package_manifest": safe_file(request["package_manifest_path"]),
        "model_manifest": safe_file(request["model_manifest_path"]),
        "runtime_manifest": safe_file(request["runtime_manifest_path"]),
        "runtime_python": safe_file(request["runtime_python"]),
        "runtime_tree_manifest": safe_file(request["runtime_tree_manifest_path"]),
        "base_tree_manifest": safe_file(request["base_tree_manifest_path"]),
        "wheel_verification": safe_file(request["wheel_verification_path"]),
        "host_preflight": safe_file(request["host_preflight_path"]),
        "conformance_request": safe_file(request["conformance_request_path"]),
    }
    expected_labels = set(input_paths)
    if (
        not isinstance(request["input_file_sha256"], dict)
        or set(request["input_file_sha256"]) != expected_labels
    ):
        raise AdapterError("adapter-check input hash allowlist mismatch")
    for label, path in input_paths.items():
        expected = request["input_file_sha256"][label]
        if not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise AdapterError("adapter-check input digest is malformed")
        exact_file_hash(path, expected, label)
    if Path(sys.executable).resolve(strict=True) != input_paths["runtime_python"]:
        raise AdapterError("adapter-check interpreter mismatch")

    package_raw = input_paths["package_manifest"].read_bytes()
    package_manifest = strict_json_file(package_raw)
    exact_object(
        package_manifest,
        {
            "schema_version", "corpus_sha256", "protocol_sha256",
            "contexts_sha256", "schedule_sha256", "result_schema_sha256",
            "source_registry_sha256", "package_file_count", "provider_calls",
            "execution_authorized",
        },
        "package manifest",
    )
    if (
        package_manifest["schema_version"] != SCHEMA + "/package-manifest"
        or package_manifest["protocol_sha256"] != request["protocol_sha256"]
        or package_manifest["schedule_sha256"] != request["schedule_sha256"]
        or package_manifest["provider_calls"] != 0
        or package_manifest["execution_authorized"] is not False
        or sha256(package_raw) != request["package_manifest_sha256"]
    ):
        raise AdapterError("package manifest binding mismatch")

    model_path = safe_directory(request["model_directory"])
    model_manifest = verify_model(model_path, input_paths["model_manifest"])
    if (
        model_manifest["manifest_sha256"] != request["model_manifest_sha256"]
        or model_manifest["artifact_sha256"] != request["model_artifact_sha256"]
    ):
        raise AdapterError("model manifest binding mismatch")
    expected_runtime = strict_json_file(input_paths["runtime_manifest"].read_bytes())
    actual_runtime = runtime_manifest(
        argparse.Namespace(
            runtime_tree_manifest=request["runtime_tree_manifest_path"],
            base_tree_manifest=request["base_tree_manifest_path"],
            wheel_verification=request["wheel_verification_path"],
        )
    )
    if expected_runtime != actual_runtime:
        raise AdapterError("runtime tree differs from the frozen runtime manifest")
    if (
        expected_runtime["manifest_sha256"] != request["runtime_manifest_sha256"]
        or expected_runtime["full_tree_sha256"] != request["runtime_full_tree_sha256"]
    ):
        raise AdapterError("runtime manifest binding mismatch")

    host_raw = input_paths["host_preflight"].read_bytes()
    host = strict_json_file(host_raw)
    allowed_host_keys = {
        "schema_version", "collected_at", "os_version", "os_build",
        "architecture", "chip", "physical_memory_bytes", "vm_available_percent",
        "memory_pressure_free_percent", "swap_used_bytes", "disk_free_bytes",
        "power_source", "sandbox_exec_available", "metal_trace_available",
        "heavy_processes", "eligible", "failures",
    }
    if "xctrace_version" in host:
        allowed_host_keys.add("xctrace_version")
    exact_object(host, allowed_host_keys, "host preflight")
    if (
        host["schema_version"] != SCHEMA + "/host-preflight"
        or host["eligible"] is not True
        or host["failures"] != []
        or sha256(host_raw) != request["host_preflight_sha256"]
    ):
        raise AdapterError("host preflight binding mismatch")

    conformance_raw = input_paths["conformance_request"].read_bytes()
    conformance_request = strict_json_file(conformance_raw)
    if sha256(conformance_raw) != request["conformance_request_sha256"]:
        raise AdapterError("conformance request binding mismatch")
    conformance = framing_conformance(conformance_request)

    output = Path(request["output_namespace"])
    if (
        not output.is_absolute()
        or output.exists()
        or sha256(str(output).encode()) != request["output_namespace_sha256"]
    ):
        raise AdapterError("output namespace binding mismatch")
    for forbidden_parent in (repository, model_path, Path(sys.executable).parent.parent):
        try:
            output.relative_to(forbidden_parent)
        except ValueError:
            pass
        else:
            raise AdapterError("output namespace overlaps a trusted input")

    completed = datetime.now(timezone.utc)
    if completed >= expires:
        raise AdapterError("adapter-check expired during execution")
    completed_text = completed.isoformat(timespec="microseconds").replace("+00:00", "Z")
    receipt = {
        "schema_version": SCHEMA + "/adapter-check-receipt",
        "request_sha256": request["request_sha256"],
        "started_at": request["started_at"],
        "completed_at": completed_text,
        "expires_at": request["expires_at"],
        "implementation_commit": request["implementation_commit"],
        "implementation_tree": request["implementation_tree"],
        "build_package": request["build_package"],
        "build_go_version": request["build_go_version"],
        "build_vcs_revision": request["build_vcs_revision"],
        "build_vcs_modified": request["build_vcs_modified"],
        "input_file_sha256": request["input_file_sha256"],
        "package_manifest_sha256": request["package_manifest_sha256"],
        "protocol_sha256": request["protocol_sha256"],
        "schedule_sha256": request["schedule_sha256"],
        "model_manifest_sha256": request["model_manifest_sha256"],
        "model_artifact_sha256": request["model_artifact_sha256"],
        "runtime_manifest_sha256": request["runtime_manifest_sha256"],
        "runtime_full_tree_sha256": request["runtime_full_tree_sha256"],
        "runtime_interpreter_sha256": request["input_file_sha256"]["runtime_python"],
        "adapter_sha256": request["input_file_sha256"]["adapter"],
        "tool_sha256": request["input_file_sha256"]["tool"],
        "host_preflight_sha256": request["host_preflight_sha256"],
        "conformance_request_sha256": request["conformance_request_sha256"],
        "framing_conformance_sha256": conformance["conformance_sha256"],
        "framing_conformance_vectors": len(conformance["results"]),
        "output_namespace_sha256": request["output_namespace_sha256"],
        "adapter_check_directory_sha256": request["adapter_check_directory_sha256"],
        "model_loaded": False,
        "network_allowed": False,
        "receipt_sha256": "",
    }
    if any(name == "mlx" or name.startswith("mlx.") or name == "mlx_lm" or name.startswith("mlx_lm.") for name in sys.modules):
        raise AdapterError("adapter-check imported model runtime modules")
    receipt["receipt_sha256"] = domain_digest("adapter-check-receipt", receipt)
    return receipt


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser()
    commands = root.add_subparsers(dest="command", required=True)
    commands.add_parser("classifier-self-test")
    commands.add_parser("framing-conformance")
    check = commands.add_parser("adapter-check")
    check.add_argument("--request", required=True)
    manifest = commands.add_parser("runtime-manifest")
    manifest.add_argument("--trusted-repo-root", required=True)
    manifest.add_argument("--expected-commit", required=True)
    manifest.add_argument("--expected-self-sha256", required=True)
    serve_command = commands.add_parser("serve")
    for command in (serve_command,):
        command.add_argument("--trusted-repo-root", required=True)
        command.add_argument("--expected-commit", required=True)
        command.add_argument("--expected-self-sha256", required=True)
    serve_command.add_argument("--model-path", required=True)
    serve_command.add_argument("--model-manifest", required=True)
    serve_command.add_argument("--runtime-manifest", required=True)
    serve_command.add_argument("--package-manifest", required=True)
    serve_command.add_argument("--authorization", required=True)
    serve_command.add_argument("--adapter-check-receipt", required=True)
    serve_command.add_argument("--output-namespace", required=True)
    serve_command.add_argument("--run-namespace", required=True)
    serve_command.add_argument("--run-manifest", required=True)
    for command in (manifest, serve_command):
        command.add_argument("--runtime-tree-manifest", required=True)
        command.add_argument("--base-tree-manifest", required=True)
        command.add_argument("--wheel-verification", required=True)
    return root


def main() -> None:
    args = parser().parse_args()
    if args.command == "classifier-self-test":
        vectors = strict_json_frame(sys.stdin.buffer.read(), require_object=False)
        if not isinstance(vectors, list):
            raise AdapterError("classifier vectors must be an array")
        output = []
        for vector in vectors:
            if not isinstance(vector, dict) or set(vector) != {"raw_base64", "allowed"}:
                raise AdapterError("classifier vector mismatch")
            raw = base64.b64decode(vector["raw_base64"], validate=True)
            output.append(parse_projection(raw, vector["allowed"])["parse_status"])
        write_frame(output)
        return
    if args.command == "framing-conformance":
        request = strict_json_frame(sys.stdin.buffer.read())
        write_frame(framing_conformance(request))
        return
    if args.command == "adapter-check":
        write_frame(adapter_check(args))
        return
    repository = safe_directory(args.trusted_repo_root)
    verify_self(repository, args.expected_commit, args.expected_self_sha256)
    if args.command == "runtime-manifest":
        write_frame(runtime_manifest(args))
        return
    serve(args)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"local-context-control-mlx-v2: {type(exc).__name__}: {exc}", file=sys.stderr)
        raise SystemExit(1)
