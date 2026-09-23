# Local Context-Control Prospective Amendment v1

**Status:** prospective; frozen before any local-model observation

**Amended:** 2026-09-22

**Base protocol:** `local-control-preregistration-v1.md`

No model was loaded and no study observation existed when this amendment was
frozen. It does not alter the TypeSafe final study or admit any held-out
AgentTrace evidence.

## Runtime custody clarification

The eligible CPython 3.13.15 runtime is now fully frozen. The venv tree permits
no `.pth`, `sitecustomize.py`, `usercustomize.py`, `__pycache__`, `.pyc`,
`.pyo`, or unregistered package symlink. The external base CPython tree may
contain standard-library bytecode, but every file, directory, symlink target,
mode, and the interpreter binary are frozen by a complete base-tree manifest.

Bindings:

- package closure:
  `119a039f08656f87e0a847a2ec9d9ba0633c12418fd5e08c469e67a262e46997`;
- requirements input:
  `4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b`;
- hash-locked requirements:
  `779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1`;
- wheel RECORD verification:
  `b628d0b295cb4ce1cac3c51489930281581b9de4b7ad8e168d2e154a6cc98d3a`;
- venv tree:
  `0941d4489820fffe045875d9e192378af135c8d0ca7d1d6c4f4b240d74b0f222`;
- base CPython tree:
  `295be0913c3d0390eccd7ffbc456874677a8e7b417de8bd90cae13102cc5a9d5`;
- base interpreter binary:
  `4e1dfb03f82c5f7f253bbc3c04a79bb9f09a5cd0528829c32d6984ef309ebb2f`.

The current runtime status is
`fresh_runtime_frozen_execution_requires_clean_host_preflight`. Trusted Go
code verifies both trees before Python executes and after observations.

## Structural routing selection

Candidate rules are evaluated on pooled Arm A and Arm C Distill observations.
Select the highest equal-base-weighted coverage whose observed unsafe risk is
at most `0.15` with at least `3` accepted observations. Ties retain the frozen
strict-to-lenient order:

1. `accept_with_all_evidence`;
2. `accept_with_any_evidence`;
3. `generated_accept`.

Per-arm Distill rows are diagnostic and cannot select the rule. Evaluate
exactly the selected rule once on LLMTraceFX, reporting pooled and per-arm
diagnostic rows. If no pooled Distill rule qualifies, report
`no-safe-auto-action` and expose no LLMTraceFX candidate curve.

## Repeatability denominator clarification

Failures remain in decision, route, malformed, latency, and token-dispersion
denominators. Citation Jaccard is conditioned on triples in which all three
scheduled observations parsed validly. The result reports
`citation_triples_evaluated` and `citation_nonempty_union_triples`; an all-valid
triple with three empty citation sets scores `1.0` by convention, while
`mean_nonempty_evidence_id_jaccard` excludes empty-union triples.

## Interpretation clarifications

The task exposes the decision rubric and machine-readable facts. If Arm A
correctness is near `1.0`, the comparison is saturated and weakly informative;
a null delta does not show compilation cannot help harder tasks.

Distill Lock exact de-duplication retains one lexical representative path for
identical normalized bytes. Arm C may preserve evidence content while omitting
one duplicate registered path label. Citation-set and
`accept_with_all_evidence` comparisons therefore partly measure compiler
evidence labeling; they remain secondary and cannot redefine correctness.
