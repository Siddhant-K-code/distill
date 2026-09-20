# Related Work

**Retrieval date for every URL:** 2026-09-20

This matrix distinguishes specifications and company claims from independent
findings. A first-party source establishes what a project or vendor says and
how it defines an interface; it is not treated as independent evidence that
the claimed downstream effect holds. No source below is ground truth for this
study's cases.

## Context engineering and reproducible artifacts

| Source | Evidence type | Finding relevant to this protocol | Limitation |
|---|---|---|---|
| [Distill Lock v0 specification](https://github.com/Siddhant-K-code/distill/blob/a9b14667024c27c32c9c9dfb9c8dc35865990979/docs/distill-lock-v0.md), Distill, frozen at merge commit `a9b1466` | **First-party project specification** | Defines the intervention: an explicit source inventory, UTF-8 NFC/LF normalization, exact de-duplication, deterministic selection/rendering, content hashes, atomic publication, and offline verification. | It specifies behavior; it is not independent evidence that deterministic context improves decisions. It expressly does not make models, providers, retrieval, agents, or networks deterministic. |
| Lamb and Zacchiroli, [“Reproducible Builds: Increasing the Integrity of Software Supply Chains”](https://doi.org/10.1109/MS.2021.3073045), *IEEE Software* 39(2), 2022 | Peer-reviewed synthesis | Repeated builds from specified sources and dependencies should yield bit-for-bit identical artifacts that can be compared by digest. This is the closest established analogy for compiling context. | Reproducibility establishes source-to-artifact correspondence, not source correctness or downstream decision correctness. |
| Rundgren, Jordan, and Erdtman, [RFC 8785: JSON Canonicalization Scheme](https://doi.org/10.17487/RFC8785), 2020 | IETF technical specification | Deterministic property ordering and primitive serialization make JSON suitable for reproducible hashing and signing. | The informational RFC does not define Distill's Unicode, filesystem, source-selection, or build semantics. |
| Whistler, [Unicode Standard Annex #15: Unicode Normalization Forms, Revision 57](https://www.unicode.org/reports/tr15/tr15-57.html), Unicode 17.0.0, 2025 | Normative standard | NFC gives canonically equivalent Unicode strings a common normalized representation. | NFC does not normalize line endings, whitespace, paths, metadata, or meaning. |
| Anthropic Engineering, [“Effective context engineering for AI agents”](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents), 2025 | **Company guidance** | Defines context engineering as curating the full inference-time token set and recommends a small, high-signal context. | Qualitative vendor guidance; it does not show that a particular compiler is reproducible or improves calibration. |

## Calibration, proper scores, and selective prediction

| Source | Evidence type | Finding relevant to this protocol | Limitation |
|---|---|---|---|
| Brier, [“Verification of Forecasts Expressed in Terms of Probability”](https://doi.org/10.1175/1520-0493%281950%29078%3C0001%3AVOFEIT%3E2.0.CO%3B2), *Monthly Weather Review* 78(1), 1950 | Foundational peer-reviewed paper | Introduces the quadratic probability score now called the Brier score. | Brier score reflects calibration and resolution/discrimination; it is not a calibration-only measure, and multiclass scaling conventions differ. |
| Good, [“Rational Decisions”](https://doi.org/10.1111/j.2517-6161.1952.tb00104.x), *JRSS B* 14(1), 1952 | Foundational decision-theory paper | Develops the logarithmic scoring rule underlying log loss. | Log loss is unbounded and heavily penalizes very small probability on the realized outcome. |
| Gneiting and Raftery, [“Strictly Proper Scoring Rules, Prediction, and Estimation”](https://doi.org/10.1198/016214506000001437), *JASA* 102(477), 2007 | Peer-reviewed theoretical review | Strictly proper scores incentivize reporting the forecaster's true distribution and justify using quadratic and logarithmic scores. | Proper scores assess total probability quality, not calibration alone. |
| Guo et al., [“On Calibration of Modern Neural Networks”](https://proceedings.mlr.press/v70/guo17a.html), ICML 2017 | Peer-reviewed empirical paper | Demonstrates that accuracy and confidence calibration can diverge, motivating separate outcomes. | Studies supervised classifiers and mostly in-distribution post-hoc calibration, not agentic evidence decisions under shift. |
| Nixon et al., [“Measuring Calibration in Deep Learning”](https://openaccess.thecvf.com/content_CVPRW_2019/html/Uncertainty_and_Robustness_in_Deep_Visual_Learning/Nixon_Measuring_Calibration_in_Deep_Learning_CVPRW_2019_paper.html), CVPR Workshops 2019 | Peer-reviewed workshop paper | Shows that conventional top-label ECE omits non-argmax probabilities and changes with bin count, range, and discretization. | Alternative ECE definitions address different weaknesses; no one variant is universally definitive. |
| Roelofs et al., [“Mitigating Bias in Calibration Error Estimation”](https://proceedings.mlr.press/v151/roelofs22a.html), AISTATS 2022 | Peer-reviewed statistical study | Finds finite-sample bias and bin sensitivity in standard ECE and evaluates equal-mass, debiased, and sweep estimators. | Performance depends on the calibration notion and data-generating process. |
| El-Yaniv and Wiener, [“On the Foundations of Noise-Free Selective Classification”](https://jmlr.org/papers/v11/el-yaniv10a.html), *JMLR* 11, 2010 | Peer-reviewed theoretical paper | Formalizes reject-option prediction through coverage and loss conditional on acceptance (selective risk). | Principal guarantees use realizability/noise-free assumptions and do not automatically extend to repository distribution shift. |

Together these sources support reporting actions, proper scores, calibration,
and risk/coverage separately. This protocol therefore fixes the ECE binning
method, treats ECE as secondary, preserves full distributions, and uses exact
or cluster-aware uncertainty rather than a point estimate alone.

## TypeSafe and Jev workflow evaluation

The sources in this section are TypeSafe publications. No separately authored
Jev research paper or independent reproduction of the complete workflow
benchmark was found as of the retrieval date.

| Source | Evidence type | Published claim or method | Limitation |
|---|---|---|---|
| Almeida / TypeSafe AI, [“Introducing System One Models & Jev”](https://typesafe.ai/blog/introducing-system-one-models-and-jev), 2026 | **Company launch post** | Describes fixed “workflow evals” that decompose broad policy into typed probabilistic questions while deterministic code performs arithmetic, branching, and actions. The reference probabilities are described as an average of two frontier models. | Reported intelligence, calibration, speed, cost, and hallucination properties are vendor claims. Frontier-model-consensus references are not independently adjudicated truth. The post acknowledges workflow-author bias and unusually favorable speed/cost conditions. |
| TypeSafe AI, [“Workflow evals”](https://evals.typesafe.ai/), undated dynamic site | **Company benchmark site** | Presents Jev comparisons on company-constructed workflows and scores agreement with actions derived from frontier-model-consensus probabilities. | The complete case corpus, executable harness, scoring implementation, uncertainty intervals, and repeated-run analysis were not found. The page is mutable, and its “accuracy” is agreement with a proxy reference. |
| TypeSafe AI, [“System One” documentation](https://docs.typesafe.ai/concepts/system-one.md), undated | **Company product documentation** | Defines a state-plus-typed-question interface returning constrained values and probabilities for deterministic policy code to combine. It notes that calibration is a group property, not a per-answer guarantee. | This is not a model card, calibration study, peer-reviewed method, or independent evaluation. Public training details, held-out calibration curves, subgroup analysis, and a formal RLCD algorithm were not found. |

The design adopts the useful separation between probabilistic atomic questions
and ordinary policy code, but does not adopt TypeSafe's consensus labels as
ground truth. A Jev arm remains contingent on an immutable version, documented
probability semantics, a stable SDK/API, usage and price fields, rate limits,
known jagged edges, and publication independence.

### Jev evidence gap

The search found no public:

- peer-reviewed paper or preprint describing Jev's architecture or training
  objective;
- model card with training data, calibration curves, or subgroup analysis;
- complete workflow-evaluation corpus and executable harness;
- human or expert ground-truth adjudication for all workflow cases; or
- independent reproduction of the published workflow results.

Absence from this search is not proof that an artifact does not exist. Any
later artifact must be dated, archived, and reviewed before it can satisfy an
execution gate.

## Coding-agent verification and software evidence

| Source | Evidence type | Finding relevant to this protocol | Limitation |
|---|---|---|---|
| Jimenez et al., [“SWE-bench: Can Language Models Resolve Real-World GitHub Issues?”](https://arxiv.org/abs/2310.06770v3), ICLR 2024 Oral | Peer-reviewed benchmark paper | Evaluates patches for real public GitHub issues in executable repository environments, supporting tests and repository state as non-model evidence. | The static corpus is Python-heavy, and passing the selected tests is not proof of complete semantic correctness. |
| OpenAI and the SWE-bench authors, [“Introducing SWE-bench Verified”](https://openai.com/index/introducing-swe-bench-verified/), 2024, updated 2025 | **Benchmark-owner/company report** | Reports a multi-annotator curation process that removed many underspecified or unfair tasks. | Curation procedures and model results are owner-reported rather than an independent replication. |
| Mündler et al., [“SWT-Bench: Testing and Validating Real-World Bug-Fixes with Code Agents”](https://doi.org/10.52202/079017-2601), NeurIPS 2024 | Peer-reviewed benchmark paper | Requires an issue-reproducing regression test that fails before and passes after a reference fix, adding executable evidence beyond patch generation. | Generated tests may be incomplete, overfit, or encode the wrong issue interpretation. |
| Wang, Liu, and Pradel, [“Are ‘Solved Issues’ in SWE-bench Really Solved Correctly? An Empirical Study”](https://doi.org/10.1145/3744916.3764576), ICSE 2026 | **Independent peer-reviewed audit** | Finds that benchmark acceptance can miss failures revealed by full developer suites or differential behavior, supporting multiple evidence channels. | Behavioral difference from a developer patch is suspicious evidence, not automatically an error; multiple implementations can be valid. |

These findings motivate preserving the exact test commands, verifier outputs,
lifecycle state, cleanup and external-effect records, and their limits. No
single test result is elevated beyond the scope of the evidence it checks.

## Why frontier-model consensus is not ground truth

| Source | Evidence type | Independent finding | Limitation |
|---|---|---|---|
| Kim et al., [“Correlated Errors in Large Language Models”](https://proceedings.mlr.press/v267/kim25e.html), ICML 2025 | **Independent peer-reviewed empirical study** | Errors remain correlated across many models, including models from different providers and architectures. Cross-model agreement is therefore not independent corroboration. | Correlation is task- and model-population-specific and does not imply that every consensus answer is wrong. |
| Chen et al., [“Two Failures of Self-Consistency in the Multi-Step Reasoning of LLMs”](https://openreview.net/forum?id=5nBqY1y96B), *TMLR*, 2024 | Independent reviewed paper | Semantically connected reformulations and substitutions can produce contradictory answers, so repeated or majority answers need not arise from coherent computation. | Tests selected controlled transformations and earlier model generations. |
| Lin, Hilton, and Evans, [“TruthfulQA: Measuring How Models Mimic Human Falsehoods”](https://aclanthology.org/2022.acl-long.229/), ACL 2022 | Independent peer-reviewed benchmark paper | Models can reproduce common misconceptions as fluent answers, showing how shared training text can induce shared errors. | The adversarial benchmark covers older systems; its absolute scores should not be projected onto later models. |
| Wang et al., [“Large Language Models are not Fair Evaluators”](https://aclanthology.org/2024.acl-long.511/), ACL 2024 | Independent peer-reviewed empirical study | Reversing candidate order can reverse LLM-as-judge outcomes even when prompts instruct the judge to ignore order. | Balanced presentation can reduce but does not prove elimination of evaluator bias. |
| Hong et al., [“Measuring Sycophancy of Language Models in Multi-turn Dialogues”](https://aclanthology.org/2025.findings-emnlp.121/), Findings of EMNLP 2025 | Independent peer-reviewed empirical study | Models from multiple families can change positions under sustained user pressure, so shared prompts can induce shared accommodation rather than evidence-based agreement. | Some scoring uses another model as classifier, adding a judge-model limitation. |

For this study, frontier consensus may be retained only as a clearly labelled
proxy or comparator. Primary ground truth must come from independently
observable tests, verifier outcomes, lifecycle and cleanup state, external
effects, or blinded adjudication tied to those records.
