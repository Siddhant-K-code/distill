# Local context-control operator contract

This harness prepares and verifies a provider-independent local-model
development/calibration study. It is separate from the frozen TypeSafe final
study and excludes all held-out AgentTrace records.

## Offline package

```bash
make distill-local-context-control
```

This prepares, validates, and summarizes the 14-base, 118-condition,
332-observation-per-model package without network access or model execution.

## Execution prerequisites

Execution is intentionally blocked until this implementation is merged and a
clean execution session starts from the exact merged `main` commit.

1. Recreate a fresh CPython 3.13.15 runtime with the exact 34 frozen
   distributions. Do not reuse the retired runtime identity documented in the
   preregistration.
2. Build `distill-local-context-control` from clean merged main.
3. Create an owner-only model manifest with `model-manifest`.
4. Create an owner-only full-tree runtime manifest with `runtime-manifest`.
   Pass the private venv-tree manifest, base-CPython-tree manifest, and wheel
   verification record. This command is deny-networked and uses `-I -B`.
5. Run `preflight`. Any unrelated process at or above 1 GiB RSS blocks
   execution.
6. Run `authorize` with the exact acknowledgement printed below. Authorization
   binds the executable, adapter, merged commit, package, model, fresh runtime,
   and an absent private output namespace. Pass the exact externally reviewed
   merged commit and tree IDs; a locally discovered moving ref is insufficient.
7. Run the exact `run` command once. There is no retry flag.
8. Use `verify-results` and `summarize-results` in a separate result session.

Exact acknowledgement:

```text
I authorize only the pinned local 4B development/calibration schedule with zero network access, zero cloud spend, and no scaling control.
```

The model, runtime, authorization, and run paths must be absolute private paths
outside the repository. Never commit them. The runner refuses a dirty or
unmerged checkout, a changed runtime tree, bytecode/startup hooks, model
manifest drift, an existing output namespace, network-capable execution, host
identity drift, memory pressure, swap/disk/RSS limit failures, missing
receipts, or process cleanup failure.

Authorization is path-bound and permits one attempt while its ledger remains under
append-only custody. The runner locks one no-follow file descriptor, validates that
the directory entry still names the same owner-only inode, appends and syncs the sole
attempt before model load, revalidates the inode, and syncs the parent directory. If startup,
integrity, resource, timeout, or adapter failure follows, the private run
namespace is preserved with `aborted.json`; that authorization cannot be
reused. Recovery requires a newly reviewed authorization and a fresh output
namespace. Never delete or move an unfavorable attempt to rerun it.

An offline file cannot cryptographically prevent its owner from restoring a prior
filesystem snapshot. Such rollback is outside the executable's threat model and
invalidates the study. Independent custody must retain the authorization ledger and
run namespace from before model load through result review; public results must
disclose the custody record rather than claiming malicious-owner rollback resistance.

If result verification fails, `verify-results` and `summarize-results` exit non-zero
and emit no result summary. That hard error is the prospective **invalid** outcome;
`invalid` is not serialized as a successful summary field.

`run` performs the 332 real observations and therefore must not be called from
this implementation PR.
