# Distill Lock v0

Status: frozen specification for schema `distill-lock/v0`.

Distill Lock treats context as a build artifact. Its guarantee is:

> Same locked sources + same configuration + same supported runtime/tool
> identities produce byte-identical context bundle, manifests, and digests.

This is a deterministic envelope around later probabilistic systems. It does
not make model, provider, retrieval, agent, or network behavior deterministic.
Distill Lock performs no model or network calls.

## Scope and identities

The v0 tool identity is `github.com/Siddhant-K-code/distill/distill-lock-v0`
and the supported runtime identity is `go1.24-go1.26`. Lock, build, and verify
reject binaries built with other Go runtime versions. Portable artifacts bind
those identities plus these independently versioned algorithms:

| Concern | Identity |
|---|---|
| Canonicalization | `utf8-nfc-lf-v1` |
| Chunking | `utf8-fixed-bytes-v1` |
| Token estimation | `utf8-bytes-div4-ceil-v1` |
| Exact deduplication | `sha256-normalized-exact-v1` |
| Selection | `lexical-budget-v1` |
| Bundle rendering | `markdown-bundle-v1` |
| Canonical JSON | `go-struct-json-indent-v1` |

These identities are compatibility boundaries. An implementation change that
can alter portable bytes requires a new identity or schema.

## Configuration

The input is canonical JSON with schema `distill-lock/config/v0`:

```json
{
  "schema_version": "distill-lock/config/v0",
  "source_root": "sources",
  "sources": [
    "docs/overview.md",
    "src/example.go"
  ],
  "exclude": [
    "notes/ignored.txt"
  ],
  "chunk_bytes": 4096,
  "token_budget": 8192,
  "metadata_removal": "none"
}
```

Unknown or duplicate JSON fields, non-canonical serialization, unsupported
schema values, non-integer or out-of-range numbers, and trailing data are
errors. `source_root`, `sources`, and `exclude` contain normalized relative
POSIX paths only. Absolute paths, empty segments, `.`, `..`, backslashes,
NUL/control bytes, and duplicate canonical paths are rejected. `sources` and
`exclude` must be disjoint. Configuration paths resolve relative to the
configuration file. The lockfile output directory must be a strict ancestor of
the source root so its recorded `source_root` remains a non-traversing relative
POSIX path and the lockfile cannot become a source input.

`sources` is an explicit allowlist. Every regular file under `source_root` is
inventoried: listed source paths are candidates, listed exclusions receive the
reason `configured_exclusion`, and all other files receive `not_listed`.
Missing configured paths are errors. A later new file is therefore observable
drift rather than an implicit input.

`chunk_bytes` is in the range 1..1048576. `token_budget` is a non-negative
integer. `metadata_removal` is fixed to `none` in v0; no metadata is removed.

## Source identity and filesystem rules

The source root identity in portable artifacts is its relative POSIX path from
the lockfile directory. Each inventory entry records:

- normalized relative POSIX path;
- original byte SHA-256 and original byte length;
- normalized-content SHA-256 and normalized byte length;
- configured inclusion state and its explicit reason.

Enumeration order from the filesystem is irrelevant. Entries are sorted by
normalized relative path using bytewise UTF-8 lexical order. Path, normalized
byte range, then digest are the deterministic chunk and selection tie-breakers.

Portable artifacts never contain absolute paths, cwd, timestamps, hostname,
username, inode, permissions, environment variables, or other machine-specific
values.

Only regular files with supported text/code names or extensions are accepted.
Directories are traversed. Devices, sockets, FIFOs, and other special files are
rejected. V0 rejects every symbolic link, including an in-root target, rather
than depending on platform-specific link resolution. An escaping symlink
therefore also fails closed. Hard-linked regular files are treated as separate
paths and exact-content deduplication handles them normally.

The source, configuration, lockfile, output, and staging paths form a local
trust boundary. Their ancestors and source directories must be owned by the
current user or root and must not be group/world writable unless the directory
has the sticky bit. Source/config/lock files must not be group/world writable.
Extended ACLs are rejected, including inherited ACLs on staging outputs. These
checks prevent another OS principal from swapping a validated path before it is
read or published. Processes running as the same OS user are trusted; v0 is not
a same-account sandbox.

Supported extensions are `.bash`, `.c`, `.cc`, `.cfg`, `.conf`, `.cpp`, `.cs`,
`.css`, `.csv`, `.go`, `.graphql`, `.h`, `.hpp`, `.html`, `.ini`, `.java`,
`.js`, `.jsx`, `.json`, `.md`, `.php`, `.proto`, `.py`, `.rb`, `.rs`, `.scss`,
`.sh`, `.sql`, `.toml`, `.ts`, `.tsx`, `.txt`, `.xml`, `.yaml`, `.yml`, and
`.zsh`. Extensionless text files are supported. Other types are rejected, not
silently skipped or rewritten.

## Canonicalization

Each original file must be valid UTF-8 and contain no NUL byte. Invalid or
binary input is an explicit error, whether selected or excluded.

Canonicalization is exactly:

1. decode valid UTF-8;
2. normalize Unicode to NFC;
3. replace CRLF with LF, then remaining CR with LF.

No other transformation occurs. Leading, trailing, and interior whitespace is
meaningful and preserved. Final-newline presence or absence is preserved.
Binary files are never decoded with replacement characters. Original and
normalized hashes remain separate, so LF, CRLF, and CR originals may share a
normalized identity while retaining distinct source identities.

## Chunking, exact deduplication, and selection

V0 does not reuse Distill's semantic embedding/clustering pipeline. That
existing functionality remains unchanged. Lock v0 deliberately performs exact
duplicate removal only: no embeddings, semantic similarity, near-duplicate
logic, vector database, or model call.

Normalized files are split into consecutive chunks of at most `chunk_bytes`
bytes. A boundary backs up to the start of a UTF-8 code point, so chunks are
valid UTF-8; if one code point exceeds the configured limit, that code point is
the chunk. Empty files produce one zero-length chunk. Each chunk records its
normalized half-open byte range, SHA-256, estimated tokens, selection state,
and explicit reason.

The token estimate is `ceil(normalized UTF-8 byte length / 4)`, with an empty
chunk costing zero. Candidates are ordered by path, start byte, end byte, then
digest. For an identical chunk digest and byte length, the first candidate is
selected as the representative and later candidates receive
`exact_duplicate_of:<path>:<start>-<end>`. Remaining unique chunks are selected
in order while their complete token count fits the remaining budget; later
chunks receive `token_budget_exceeded`. Chunks are never partially selected.
Every source and chunk has an inclusion/exclusion reason.

## Portable outputs

`distill build` atomically publishes a fresh directory containing exactly:

```text
context.bundle.md
context.lock.json
context.manifest.json
SHA256SUMS
```

`context.lock.json` is the canonical input lockfile copied byte-for-byte.
`context.bundle.md` is rendered by `markdown-bundle-v1` in selected chunk
order. It includes deterministic chunk headers and the exact normalized chunk
bytes; wrapper newlines do not alter the chunk identities recorded in the
manifest.

`context.manifest.json` binds:

- schema and all algorithm/tool/runtime identities;
- configuration digest;
- lockfile digest;
- complete source inventory and normalized identities;
- every chunk identity, byte range, token count, order, selection state, and
  reason;
- bundle byte length and SHA-256;
- aggregate source, duplicate, selected, and token facts.

The manifest intentionally does not hash itself. `SHA256SUMS` contains lowercase
SHA-256 and byte length for `context.bundle.md`, `context.lock.json`, and
`context.manifest.json`, sorted by filename. This avoids self-referential
hashes while binding every other output.

### Canonical JSON

All v0 JSON is UTF-8 without a BOM, uses the frozen struct field order, two-space
indentation, no insignificant trailing spaces, and exactly one LF after the
final `}`. Object keys therefore appear in schema order; arrays retain their
specified deterministic order. Strings use Go `encoding/json` escaping with
HTML escaping disabled. Only bounded base-10 integers are permitted; floats,
exponents, NaN, and infinities are absent. Unknown fields are errors. A JSON
artifact is canonical only when strict decoding followed by v0 encoding
reproduces its bytes exactly.

## Commands and failure behavior

```bash
distill lock <config> --output <lockfile>
distill build <lockfile> --output <directory>
distill verify <directory>
distill verify <directory> --expected-lock-sha256 <trusted-digest>
```

- `lock` resolves the configuration, validates and freezes the complete source
  inventory, canonicalizes and chunks candidates, performs exact deduplication
  and budget selection, then atomically writes a new lockfile. It performs no
  network or model call.
- `build` strictly re-reads every source under the recorded root and refuses
  missing, changed, unsupported, duplicate, unsafe, or newly unexpected input;
  configuration, tool, runtime, and algorithm identities must still match. It
  never relocks. Output must be a safe, nonexistent directory outside the
  source root. Files are written with ordinary portable permissions to a
  sibling temporary directory, synchronized, and published with the native
  atomic no-replace rename (`renameat2(RENAME_NOREPLACE)` on Linux or
  `renamex_np(RENAME_EXCL)` on macOS) only after all hashes are complete.
  The staging directory is verified against the intended lock digest before
  publication. Publication is the final commit point, so failure or
  interruption cannot replace or leave a success-shaped destination.
- `verify` is standalone and offline. It reads only the output directory and
  validates the exact allowlisted file set, regular-file/no-symlink rules,
  schemas and identities, canonical JSON, every recorded hash and length,
  configuration/lock/manifest/source/chunk/bundle relationships, exact bundle
  regeneration, and `SHA256SUMS`. Unexpected files or any mismatch fail. By
  itself this proves consistency, not authenticity: when the directory may be
  attacker-replaceable, `--expected-lock-sha256` must supply a lowercase
  SHA-256 obtained through a trusted out-of-band channel. That trusted lock
  anchors selected chunk identities and therefore the regenerated bundle.

All failures return nonzero with a specific error. There are no warnings that
substitute for required validation, silent skips, automatic relocking,
success-shaped fallbacks, or environment-derived configuration.

## Non-goals

V0 is not semantic deduplication, a vector database, an agent framework, a model
adapter, decision replay, a dashboard, or a guarantee of deterministic model
output. It neither integrates Jev nor calls a provider.

Roadmap research may use the working article title: *Context should be a build
artifact, not a prompt assembled at runtime.* The article itself is outside v0.
