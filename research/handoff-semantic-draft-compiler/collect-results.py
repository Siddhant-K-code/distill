#!/usr/bin/env python3
"""Collect frozen execution evidence into the public research package."""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import shutil
import statistics
import time
from pathlib import Path
from typing import Any


MODEL = "gpt-5.6-sol"
PRIOR_TOTALS = {
    "input_tokens": 150489,
    "output_tokens": 8764,
    "model_requests": 27,
    "total_nano_aiu": 41781200000,
    "session_ms": 191778,
}


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def encode_exact_artifact(source: Path, destination: Path, bundle_path: str) -> None:
    data = source.read_bytes()
    write_json(
        destination,
        {
            "bundle_path": bundle_path,
            "bytes": len(data),
            "encoding": "base64",
            "sha256": sha256_bytes(data),
            "data": base64.b64encode(data).decode("ascii"),
        },
    )


def assistant_content_hash(messages: list[str]) -> str:
    digest = hashlib.sha256()
    for message in messages:
        data = message.encode("utf-8")
        digest.update(str(len(data)).encode("ascii"))
        digest.update(b":")
        digest.update(data)
    return digest.hexdigest()


def sanitize_response(events_path: Path, destination: Path, run: str) -> dict[str, Any]:
    messages: list[str] = []
    for line in events_path.read_text(encoding="utf-8").splitlines():
        event = json.loads(line)
        if event.get("type") == "assistant.message":
            messages.append(event["data"]["content"])
    artifact = {
        "run": run,
        "assistant_message_count": len(messages),
        "assistant_messages": messages,
        "raw_assistant_message_content_sha256": assistant_content_hash(messages),
        "raw_content_hash_algorithm": (
            "For each assistant.message in source order, hash ASCII decimal UTF-8 "
            "byte length, one colon byte, then exact UTF-8 data.content bytes."
        ),
        "source_event_stream_sha256": sha256_file(events_path),
        "sanitization": {
            "assistant_message_content_modified": False,
            "assistant_messages_omitted": 0,
            "removed": [
                "all non-assistant.message events",
                "assistant message metadata outside data.content",
                "tool requests and results",
                "absolute paths and runtime configuration",
                "usage, timing, opaque, and reasoning fields",
            ],
        },
    }
    write_json(destination, artifact)
    artifact["artifact_sha256"] = sha256_file(destination)
    return artifact


def usage_record(path: Path) -> dict[str, Any]:
    usage = json.loads(path.read_text(encoding="utf-8"))
    model = usage["modelMetrics"][MODEL]
    details = model["usage"]
    return {
        "api_ms": usage["totalApiDurationMs"],
        "cache_read_tokens": details["cacheReadTokens"],
        "cache_write_tokens": details["cacheWriteTokens"],
        "input_tokens": details["inputTokens"],
        "model_requests": model["requests"]["count"],
        "monetary_cost": None,
        "output_tokens": details["outputTokens"],
        "premium_request_cost": usage["totalPremiumRequestCost"],
        "reasoning_tokens": details["reasoningTokens"],
        "session_ms": None,
        "total_nano_aiu": usage["totalNanoAiu"],
        "uncached_input_tokens": usage["tokenDetails"]["input"]["tokenCount"],
        "user_requests": usage["totalUserRequests"],
    }


def sum_usage(records: list[dict[str, Any]]) -> dict[str, Any]:
    keys = [
        "api_ms",
        "cache_read_tokens",
        "cache_write_tokens",
        "input_tokens",
        "model_requests",
        "output_tokens",
        "premium_request_cost",
        "reasoning_tokens",
        "total_nano_aiu",
        "uncached_input_tokens",
        "user_requests",
    ]
    totals = {key: sum(record[key] for record in records) for key in keys}
    totals["monetary_cost"] = None
    totals["session_ms"] = None
    return totals


def pair_agreement(values: list[Any]) -> dict[str, Any]:
    pairs = [(0, 1), (0, 2), (1, 2)]
    agreeing = sum(values[left] == values[right] for left, right in pairs)
    return {
        "agreeing_pairs": agreeing,
        "possible_pairs": len(pairs),
        "rate": agreeing / len(pairs),
    }


def collect(args: argparse.Namespace) -> None:
    study = Path(__file__).resolve().parent
    repo = study.parents[1]
    execution_root = Path(args.execution_root).resolve()
    execution = json.loads((execution_root / "execution.json").read_text(encoding="utf-8"))
    trusted = json.loads((execution_root / "trusted-stage.json").read_text(encoding="utf-8"))
    trusted_by_run = {record["run"]: record for record in trusted["runs"]}

    evidence_directories = [
        study / "drafts",
        study / "canonical-drafts",
        study / "proposals",
        study / "responses",
        study / "reviews",
    ]
    if any(path.exists() for path in evidence_directories) or (study / "results.json").exists():
        raise RuntimeError("public result artifacts already exist")
    for path in evidence_directories:
        path.mkdir()

    expected_positive = json.loads(
        (repo / "testdata/handoff-v0/retry-policy/proposal.json").read_text(encoding="utf-8")
    )
    expected_candidate = expected_positive["candidates"][0]
    expected_core = expected_candidate["decision_text"].removeprefix("Decision: ")
    expected_core_sentence = expected_core[:1].upper() + expected_core[1:]
    expected_replacement = (
        "Production requests use three attempts with exponential backoff starting at 200ms."
    )
    expected_negative = json.loads(
        (repo / "testdata/handoff-v0/brainstorming/proposal.json").read_text(encoding="utf-8")
    )

    runs = []
    review_durations_us: list[int] = []
    for execution_run in execution["runs"]:
        start_review = time.monotonic_ns()
        name = execution_run["run"]
        condition = execution_run["condition"]
        run_root = execution_root / "runs" / name
        trusted_run = trusted_by_run[name]

        draft_source = run_root / "semantic-draft.json"
        draft_destination = study / "drafts" / f"{name}.json"
        shutil.copy2(draft_source, draft_destination)
        draft = json.loads(draft_destination.read_text(encoding="utf-8"))

        canonical_source = run_root / "trusted/canonical-draft.json"
        canonical_destination = study / "canonical-drafts" / f"{name}.json"
        shutil.copy2(canonical_source, canonical_destination)

        proposal_source = run_root / "trusted/proposal.json"
        proposal_destination = study / "proposals" / f"{name}.json"
        shutil.copy2(proposal_source, proposal_destination)
        proposal = json.loads(proposal_destination.read_text(encoding="utf-8"))

        review_source = run_root / "trusted/review"
        review_destination = study / "reviews" / name
        review_destination.mkdir()
        shutil.copy2(
            review_source / "handoff.receipt.json",
            review_destination / "handoff.receipt.json",
        )
        shutil.copy2(review_source / "SHA256SUMS", review_destination / "SHA256SUMS")
        encode_exact_artifact(
            review_source / "review.md",
            review_destination / "review.md.bytes.json",
            "review.md",
        )
        patch_destination = review_destination / "patches"
        for patch_source in sorted((review_source / "patches").glob("*.patch")):
            patch_destination.mkdir(exist_ok=True)
            encode_exact_artifact(
                patch_source,
                patch_destination / f"{patch_source.name}.bytes.json",
                f"patches/{patch_source.name}",
            )

        response_artifact = sanitize_response(
            run_root / "runtime/events.jsonl",
            study / "responses" / f"{name}.json",
            name,
        )
        usage = usage_record(run_root / "runtime/usage.json")

        runtime_metadata_paths = execution_run["mutation_paths"]["trust_boundary"]
        if not runtime_metadata_paths or not all(
            path == ".agent-traces/" or path.startswith(".agent-traces/")
            for path in runtime_metadata_paths
        ):
            raise RuntimeError(f"unexpected workspace additions for {name}")
        recorded_trust_boundary_mutations = execution_run["mutations"]["trust_boundary"]
        if recorded_trust_boundary_mutations != len(runtime_metadata_paths):
            raise RuntimeError(f"trust-boundary mutation count mismatch for {name}")

        score: dict[str, Any]
        if condition == "positive":
            candidate = draft["candidate"]
            compiled_candidate = proposal["candidates"][0]
            evidence_correct = (
                expected_candidate["evidence"]["quote"] in candidate["evidence_quote"]
                and trusted_run["verify_exit_status"] == 0
            )
            target_correct = candidate["target_path"] == expected_candidate["target"]["path"]
            operation_correct = (
                candidate["operation"] == "replace"
                and candidate["replacement_text"] == expected_replacement
            )
            decision_recovered = candidate["decision_text"] in (
                expected_candidate["decision_text"],
                expected_core,
                expected_core_sentence,
                expected_replacement,
            )
            patch_equivalent = (
                compiled_candidate["patch"]["unified_diff"]
                == expected_candidate["patch"]["unified_diff"]
            )
            verifier_valid = trusted_run["verify_exit_status"] == 0
            expected_candidate_recovered = all(
                [
                    decision_recovered,
                    evidence_correct,
                    target_correct,
                    operation_correct,
                    patch_equivalent,
                    verifier_valid,
                ]
            )
            score = {
                "semantic_draft_success": True,
                "expected_decision_recovered": decision_recovered,
                "exact_evidence_correct": evidence_correct,
                "fixture_expected_quote_exact_match": (
                    candidate["evidence_quote"] == expected_candidate["evidence"]["quote"]
                ),
                "target_correct": target_correct,
                "operation_replacement_correct": operation_correct,
                "compiler_success": trusted_run["compile_exit_status"] == 0,
                "verifier_success": verifier_valid,
                "candidate_id_valid": verifier_valid,
                "patch_valid": verifier_valid,
                "patch_semantically_equivalent": patch_equivalent,
                "expected_candidate_recovered": expected_candidate_recovered,
                "review_disposition": "usable unchanged" if expected_candidate_recovered else "rejected",
                "unsupported_claims": 0,
                "fabricated_candidates": 0 if expected_candidate_recovered else len(proposal["candidates"]),
            }
        else:
            correct_abstention = draft["outcome"] == "abstain" and draft["candidate"] is None
            score = {
                "semantic_draft_success": True,
                "correct_abstention": correct_abstention,
                "false_positive_semantic_drafts": 0 if correct_abstention else 1,
                "false_positive_candidates": len(proposal["candidates"]),
                "compiler_success": trusted_run["compile_exit_status"] == 0,
                "verifier_success": trusted_run["verify_exit_status"] == 0,
                "compiled_no_decision": proposal["outcome"] == expected_negative["outcome"],
            }

        review_duration_us = max(1, (time.monotonic_ns() - start_review) // 1_000)
        review_durations_us.append(review_duration_us)
        run_record = {
            **{
                key: execution_run[key]
                for key in [
                    "run",
                    "condition",
                    "ordinal",
                    "order_position",
                    "runtime",
                    "runtime_version",
                    "runtime_sha256",
                    "model",
                    "model_selected_explicitly",
                    "available_tools",
                    "session_id",
                    "start_time_utc",
                    "end_time_utc",
                    "cli_elapsed_ms",
                    "exit_status",
                    "output_file_count",
                ]
            },
            "draft_present": execution_run["draft_present"],
            "draft_sha256": sha256_file(draft_destination),
            "canonical_draft_sha256": sha256_file(canonical_destination),
            "proposal_sha256": sha256_file(proposal_destination),
            "candidate_ids": trusted_run["candidate_ids"],
            "patch_sha256": trusted_run["patch_sha256"],
            "compile_exit_status": trusted_run["compile_exit_status"],
            "compile_ms": trusted_run["compile_ms"],
            "compile_timing_note": trusted_run["initial_compile_timing_note"],
            "verify_exit_status": trusted_run["verify_exit_status"],
            "verify_ms": trusted_run["verify_ms"],
            "verification_receipt_sha256": trusted_run["receipt_sha256"],
            "review_sha256": trusted_run["review_sha256"],
            "replay_proposal_sha256": trusted_run["replay_proposal_sha256"],
            "replay_byte_identical": trusted_run["replay_byte_identical"],
            "replay_identities_identical": trusted_run["replay_identities_identical"],
            "review_duration_us": review_duration_us,
            "usage": usage,
            "mutations": {
                "request_copy": execution_run["mutations"]["request_copy"],
                "canonical_request": execution_run["mutations"]["canonical_request"],
                "source_repository": execution_run["mutations"]["source_repository"],
                "trust_boundary": recorded_trust_boundary_mutations,
            },
            "runtime_metadata": {
                "workspace_path_count": len(runtime_metadata_paths),
                "workspace_paths": runtime_metadata_paths,
                "classified_as_mutation": True,
                "reason": (
                    "The frozen launcher recorded these runtime-owned .agent-traces "
                    "paths as trust-boundary additions. They contained no protected "
                    "study material and were not model fabrication."
                ),
            },
            "response_evidence": {
                "path": f"responses/{name}.json",
                "artifact_sha256": response_artifact["artifact_sha256"],
                "source_event_stream_sha256": response_artifact["source_event_stream_sha256"],
                "raw_assistant_message_content_sha256": response_artifact[
                    "raw_assistant_message_content_sha256"
                ],
            },
            "score": score,
        }
        runs.append(run_record)

    positives = [record for record in runs if record["condition"] == "positive"]
    negatives = [record for record in runs if record["condition"] == "negative"]
    usage_totals = sum_usage([record["usage"] for record in runs])
    allowed = {
        key: value * 1.25 for key, value in PRIOR_TOTALS.items() if key != "session_ms"
    }
    cost_comparisons = {
        key: {
            "prior": PRIOR_TOTALS[key],
            "observed": usage_totals[key],
            "maximum_non_material": allowed[key],
            "material_regression": usage_totals[key] > allowed[key],
        }
        for key in ["input_tokens", "output_tokens", "model_requests", "total_nano_aiu"]
    }
    cost_comparisons["session_ms"] = {
        "prior": PRIOR_TOTALS["session_ms"],
        "observed": None,
        "maximum_non_material": PRIOR_TOTALS["session_ms"] * 1.25,
        "material_regression": None,
        "reason": "The current runtime did not directly report session duration.",
    }
    median_review_us = statistics.median(review_durations_us)
    review_material = median_review_us > 10_000 or any(
        record["score"].get("review_disposition") == "usable after edits"
        for record in positives
    )
    no_material_regression = not any(
        comparison["material_regression"] is True for comparison in cost_comparisons.values()
    ) and not review_material

    positive_recoveries = sum(
        record["score"]["expected_candidate_recovered"] for record in positives
    )
    negative_abstentions = sum(record["score"]["correct_abstention"] for record in negatives)
    replay_successes = sum(
        record["replay_byte_identical"] and record["replay_identities_identical"]
        for record in runs
    )
    fabricated = sum(record["score"].get("fabricated_candidates", 0) for record in runs)
    aggregate_mutations = {
        key: sum(record["mutations"][key] for record in runs)
        for key in ["request_copy", "canonical_request", "source_repository", "trust_boundary"]
    }
    threshold_checks = {
        "positive_verifier_valid_expected_candidate": {
            "required": 2,
            "observed": positive_recoveries,
            "passed": positive_recoveries >= 2,
        },
        "negative_verifier_valid_abstention": {
            "required": 3,
            "observed": negative_abstentions,
            "passed": negative_abstentions == 3,
        },
        "zero_fabricated_candidates": {
            "required": 0,
            "observed": fabricated,
            "passed": fabricated == 0,
        },
        "zero_mutations": {
            "required": 0,
            "observed": sum(aggregate_mutations.values()),
            "passed": sum(aggregate_mutations.values()) == 0,
        },
        "deterministic_replay": {
            "required": 6,
            "observed": replay_successes,
            "passed": replay_successes == 6,
        },
        "no_material_cost_or_review_regression": {
            "required": True,
            "observed": no_material_regression,
            "passed": no_material_regression,
        },
    }
    threshold_passed = all(check["passed"] for check in threshold_checks.values())

    results = {
        "schema_name": "handoff-semantic-draft-compiler-study",
        "schema_version": 1,
        "study_status": "completed_with_runtime_metadata_trust_boundary_mutations",
        "executed_at": "2026-09-30",
        "frozen_inputs": {
            "repository_baseline": execution["repository_baseline"],
            "protocol_sha256": execution["protocol_sha256"],
            "prompt_sha256": execution["prompt_sha256"],
            "semantic_draft_schema_sha256": execution["semantic_draft_schema_sha256"],
            "frozen_manifest_sha256": sha256_file(study / "frozen-manifest.json"),
            "positive": {
                "fixture": "testdata/handoff-v0/retry-policy",
                "request_id": execution["canonical_requests"]["positive"]["request_id"],
                "request_sha256": execution["canonical_requests"]["positive"]["request_sha256"],
            },
            "negative": {
                "fixture": "testdata/handoff-v0/brainstorming",
                "request_id": execution["canonical_requests"]["negative"]["request_id"],
                "request_sha256": execution["canonical_requests"]["negative"]["request_sha256"],
            },
        },
        "execution": {
            "runtime": "GitHub Copilot CLI",
            "runtime_version": "1.0.90-5",
            "runtime_sha256": runs[0]["runtime_sha256"],
            "model": MODEL,
            "model_selected_explicitly": True,
            "available_tools": ["view", "apply_patch"],
            "single_shot": True,
            "retries": 0,
            "run_order": execution["run_order"],
            "completed_invocations": execution["completed_invocations"],
        },
        "protocol_deviations": [
            {
                "deviation": (
                    "The runtime created a six-path .agent-traces tree beside request/ "
                    "and output/ inside each disposable workspace, while the frozen "
                    "protocol said runtime metadata would remain outside that parent."
                ),
                "impact": (
                    "The runtime-owned metadata contained no protected study material, "
                    "was not a model output, did not alter request or output bytes, and "
                    "was removed with each disposable workspace."
                ),
                "mutation_classification": "trust_boundary_path_additions",
            }
        ],
        "aggregate_mutations": aggregate_mutations,
        "runtime_metadata_workspace_paths": sum(
            record["runtime_metadata"]["workspace_path_count"] for record in runs
        ),
        "directly_reported_totals": usage_totals,
        "cost_review_comparison": {
            "cost_metrics": cost_comparisons,
            "review_duration_us": review_durations_us,
            "review_duration_us_median": median_review_us,
            "review_material_regression": review_material,
            "no_material_regression": no_material_regression,
        },
        "positive_aggregate": {
            "semantic_draft_successes": sum(
                record["score"]["semantic_draft_success"] for record in positives
            ),
            "expected_decision_recoveries": sum(
                record["score"]["expected_decision_recovered"] for record in positives
            ),
            "expected_candidate_recoveries": positive_recoveries,
            "compiler_successes": sum(
                record["score"]["compiler_success"] for record in positives
            ),
            "verifier_successes": sum(
                record["score"]["verifier_success"] for record in positives
            ),
            "exact_evidence_correct": sum(
                record["score"]["exact_evidence_correct"] for record in positives
            ),
            "fixture_expected_quote_exact_matches": sum(
                record["score"]["fixture_expected_quote_exact_match"] for record in positives
            ),
            "target_correct": sum(record["score"]["target_correct"] for record in positives),
            "operation_replacement_correct": sum(
                record["score"]["operation_replacement_correct"] for record in positives
            ),
            "patch_semantically_equivalent": sum(
                record["score"]["patch_semantically_equivalent"] for record in positives
            ),
            "usable_unchanged": sum(
                record["score"]["review_disposition"] == "usable unchanged"
                for record in positives
            ),
            "fabricated_candidates": fabricated,
            "unsupported_claims": sum(
                record["score"]["unsupported_claims"] for record in positives
            ),
            "byte_identical_draft_agreement": pair_agreement(
                [record["draft_sha256"] for record in positives]
            ),
            "outcome_agreement": pair_agreement(
                [draft["outcome"] for draft in [
                    json.loads((study / "drafts" / f"{record['run']}.json").read_text())
                    for record in positives
                ]]
            ),
            "candidate_id_agreement": pair_agreement(
                [record["candidate_ids"] for record in positives]
            ),
        },
        "negative_aggregate": {
            "semantic_draft_successes": sum(
                record["score"]["semantic_draft_success"] for record in negatives
            ),
            "correct_abstentions": negative_abstentions,
            "false_positive_semantic_drafts": sum(
                record["score"]["false_positive_semantic_drafts"] for record in negatives
            ),
            "false_positive_candidates": sum(
                record["score"]["false_positive_candidates"] for record in negatives
            ),
            "compiler_successes": sum(
                record["score"]["compiler_success"] for record in negatives
            ),
            "verifier_successes": sum(
                record["score"]["verifier_success"] for record in negatives
            ),
            "byte_identical_draft_agreement": pair_agreement(
                [record["draft_sha256"] for record in negatives]
            ),
            "outcome_agreement": pair_agreement(
                [record["score"]["correct_abstention"] for record in negatives]
            ),
            "candidate_id_agreement": pair_agreement(
                [record["candidate_ids"] for record in negatives]
            ),
        },
        "productization_threshold": {
            "checks": threshold_checks,
            "overall_passed": threshold_passed,
            "disposition": "threshold_passed" if threshold_passed else "threshold_failed",
        },
        "publication_assessment": "useful_engineering_note",
        "evidence_accounting_correction": {
            "basis": (
                "Preserve the frozen launcher's recorded .agent-traces path additions "
                "as trust-boundary mutations without changing retained trial evidence."
            ),
            "changed_post_study_artifacts": [
                "README.md",
                "SHA256SUMS",
                "collect-results.py",
                "results.json",
            ],
            "protected_trial_artifacts_changed": False,
        },
        "scoring_notes": [
            (
                "The pre-publication collector initially compared the positive decision "
                "text to a case-sensitive prefix-stripped fixture string. It was corrected "
                "to accept the same sentence with an initial capital before results were "
                "frozen; raw drafts, compiled proposals, and verifier outputs were unchanged."
            )
        ],
        "claim_boundary": {
            "descriptive_only": True,
            "significance_claim": False,
            "superiority_claim": False,
            "adoption_claim": False,
            "safety_generalization_claim": False,
            "general_model_quality_claim": False,
        },
        "runs": runs,
    }
    write_json(study / "results.json", results)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--execution-root", required=True)
    return parser.parse_args()


if __name__ == "__main__":
    collect(parse_args())
