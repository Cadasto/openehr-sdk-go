# `resources/terminology/` — the pinned openEHR Terminology

The openEHR Foundation's computable form of the **openEHR Terminology**: the `openehr`
terminology's **groups** (*audit change type*, *version lifecycle state*, *setting*,
*participation mode*, *composition category*, …) and **code sets** (*normal statuses*,
*compression algorithms*, *integrity check algorithms*) that the Reference Model's own
invariants reference — `AUDIT_DETAILS.Change_type_valid`, `EVENT_CONTEXT.Setting_valid`,
`PARTICIPATION.Mode_valid`, `DV_ORDERED.Normal_status_validity`, and their siblings.

It is small, closed, and versioned with the specifications, so the SDK carries it the way
it carries the BMM: pinned in-tree and generated from, **not** fetched at build time and
**not** a runtime terminology-service lookup. Contract:
[`../../docs/specifications/rm-modeling.md § openEHR terminology vocabulary (REQ-034)`](../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034).

> **For AI agents and contributors:** this XML is the source of truth for openEHR codes
> and rubrics — but read it to *check* the pin, not to copy codes out of it. SDK code takes
> a code set, a rubric, and a membership verdict from the generated `openehr/terminology`
> accessor; a hand-typed openEHR code table or rubric anywhere else is a defect
> (§ REQ-034, *One home per code*). External terminologies (SNOMED CT, LOINC, …) are out
> of scope here — see [REQ-105](../../docs/specifications/clinical-modeling.md#req-105--terminology-bindings).

## The pin

| Property | Value |
|---|---|
| File | `openehr_terminology.xml` (13,860 bytes) |
| TERM release | `Release-3.0.0` — root attributes `name="openehr" language="en" version="3.0.0" date="2023-03-05"` |
| Contents | **17** groups · **3** code sets · **249** concepts · **19** code-set codes |

**Pin + integrity:** the exact upstream commit, the file's `sha256`, the release and the fetch
timestamp are recorded in [`MANIFEST.txt`](MANIFEST.txt) — the one place they live. The sync
script generates it, so do not edit it by hand. Regenerate with `make terminology-sync`; verify
with `make terminology-verify` (see [the sync script](../../scripts/sync-terminology.sh)).

## Provenance

- **Source:** [`openEHR/specifications-TERM`](https://github.com/openEHR/specifications-TERM) — `computable/XML/en/openehr_terminology.xml`.
- **Copy fidelity:** a **byte-identical** copy of the file at the pinned commit. `sync`
  resolves the ref to a concrete commit sha and downloads at that sha, so two syncs of the
  same ref produce the same bytes.
- **Language:** the `en` variant only. The sibling translations upstream (`es`, `ja`, `pt`)
  carry the same code structure with localised rubrics and are intentionally **not** vendored.

It lives in-tree rather than behind an upstream URL for the same three reasons the
[BMM pins](../bmm/README.md) do:

1. **Network determinism.** The generated accessor must be a deterministic function of its
   input; fetching at build time introduces flake and TOFU risk.
2. **Auditability.** A terminology bump is a real commit, reviewable in the PR diff.
3. **Air-gapped builds.** Consumers building behind a corporate proxy or in an air-gapped
   CI need the inputs in-tree.

## Consumers

| Consumer | Relationship |
|---|---|
| `openehr/terminology` | The generated, stdlib-only accessor — every group and code set of this pin as typed, closed, source-ordered values with code → rubric, rubric → code, and membership lookups, plus the pinned release version (§ REQ-034). |
| `cmd/termgen` | The generator: reads this pin, emits those tables. `make termgen` regenerates them; `make termgen-verify` fails the gate when they drift from the pin (the [REQ-042](../../docs/specifications/bmm-conformance.md#req-042--generated-code-drift-detected) rule). |

The generator and the accessor land with the rest of the REQ-034 work; this pin is their
only input. Nothing else in the SDK parses this XML at build or run time.

## Updating the pin

```bash
TERMINOLOGY_REF=Release-X.Y.Z make terminology-sync   # pin a named TERM release
make terminology-check                                # integrity + "is there a newer release?"
```

`terminology-sync` rewrites the XML and `MANIFEST.txt`, then runs `make termgen` so the
generated tables follow the pin. A bump is one explicit, reviewable commit:

1. Run the sync at the new release tag.
2. Review both diffs — the XML and the regenerated tables. A code or rubric that changed
   meaning, or a group that lost a member, is a behaviour change, not a refresh.
3. Update the release tag in this README's pin table and in the [`resources/README.md`](../README.md)
   inventory row — both name the pinned TERM release, and the manifest cannot update them.
4. Add a short CHANGELOG bullet under `## [Unreleased]`.
5. Commit the pin, the manifest, the regenerated tables and those doc rows together.

With no `TERMINOLOGY_REF`, `sync` re-fetches the ref already pinned in `MANIFEST.txt`
(and, with nothing pinned, the repository's latest GitHub release).

## Integrity

```bash
make terminology-verify   # offline sha256 against MANIFEST.txt; run by `make ci`
```

`terminology-verify` needs neither network nor `curl`/`jq` — only `sha256sum` or `shasum` —
so it is safe inside the gate. It reports `FAILED … (sha256 mismatch)` on any edit to the
vendored file, `MISSING` when the file is gone, and `UNTRACKED` for an XML in this directory
that the manifest does not list. **Never hand-edit the pin** — being byte-identical to
upstream is its whole value ([AGENTS.md](../../AGENTS.md) § Gotchas); re-sync instead.

## References

- Normative contract: [`rm-modeling.md § openEHR terminology vocabulary (REQ-034)`](../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034).
- Sync script: [`../../scripts/sync-terminology.sh`](../../scripts/sync-terminology.sh) (`sync` / `check` / `verify`).
- Upstream specification: openEHR *Terminology* (TERM) component — <https://specifications.openehr.org/releases/TERM/>.
