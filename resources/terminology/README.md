# `resources/terminology/` — the pinned openEHR Terminology

The openEHR Foundation's computable form of the **openEHR Terminology**, in two files:

- `openehr_terminology.xml`: the `openehr` terminology's **groups** (*audit change type*,
  *version lifecycle state*, *setting*, *participation mode*, *composition category*, …) and
  the **code sets** openEHR issues itself (*normal statuses*, *compression algorithms*,
  *integrity check algorithms*).
- `openehr_external_terminologies.xml`: the **external code sets** the Foundation publishes
  beside them: *countries* (ISO 3166-1), *character sets* (IANA), *languages* (ISO 639-1) and
  *media types* (IANA). They are the Foundation's snapshot for the release, not the live ISO
  and IANA registers.

The Reference Model's own invariants reference them: `AUDIT_DETAILS.Change_type_valid`,
`EVENT_CONTEXT.Setting_valid`, `PARTICIPATION.Mode_valid`, `DV_ORDERED.Normal_status_validity`,
`ENTRY.Language_valid`, and their siblings.

The terminology is small, closed, and versioned with the specifications. The SDK carries it
the way it carries the BMM: pinned in-tree and used as a generator input. It is not
fetched at build time and not looked up in a terminology service at run time. Contract:
[`../../docs/specifications/rm-modeling.md § openEHR terminology vocabulary (REQ-034)`](../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034).

> **For AI agents and contributors:** these XML files are the source of truth for openEHR
> codes and rubrics and for the external code sets. Read them to *check* the pin, not to
> copy codes out of them. SDK code takes code sets, rubrics, and membership verdicts from
> the generated `openehr/terminology` accessor. A hand-typed openEHR code table or rubric
> anywhere else is a defect (§ REQ-034, *One home per code*). Other external terminologies
> (SNOMED CT, LOINC, …) are out of scope here; see
> [REQ-105](../../docs/specifications/clinical-modeling.md#req-105--terminology-bindings).

## The pin

| Property | Value |
|---|---|
| Files | `openehr_terminology.xml` (13,860 bytes) · `openehr_external_terminologies.xml` (30,342 bytes) |
| TERM release | `Release-3.0.0`, both files from the same upstream commit |
| Root attributes | `openehr_terminology.xml`: `name="openehr" language="en" version="3.0.0" date="2023-03-05"` · `openehr_external_terminologies.xml`: `name="openehr" language="en" version="3.0.0" date="2023-02-11"` |
| Contents of `openehr_terminology.xml` | **17** groups · **3** code sets (issuer `openehr`) · **249** concepts · **19** code-set codes |
| Contents of `openehr_external_terminologies.xml` | **4** code sets · **624** codes: *countries* `ISO_3166-1` (250) · *character sets* `IANA_character-sets` (14) · *languages* `ISO_639-1` (253) · *media types* `IANA_media-types` (107) |

The exact upstream commit, each file's `sha256` and the fetch timestamp
are recorded in [`MANIFEST.txt`](MANIFEST.txt), and nowhere else. The sync
script generates it, so do not edit it by hand. Regenerate with `make terminology-sync`; verify
with `make terminology-verify` (see [the sync script](../../scripts/sync-terminology.sh)).

## Provenance

- **Source:** [`openEHR/specifications-TERM`](https://github.com/openEHR/specifications-TERM),
  files `computable/XML/en/openehr_terminology.xml` and
  `computable/XML/openehr_external_terminologies.xml` (the second sits directly under
  `computable/XML/`, not under a language directory).
- **Copy fidelity:** a byte-identical copy of each file at the pinned commit. `sync`
  resolves the ref to one concrete commit sha and downloads both files at that sha, so two
  syncs of the same ref produce the same bytes and the two files never come from different
  commits.
- **Language:** the `en` variant of `openehr_terminology.xml` only. The sibling translations
  upstream (`es`, `ja`, `pt`) carry the same code structure with localised rubrics and are
  intentionally not vendored. `openehr_external_terminologies.xml` has no translations.
- **Letter case:** the generated accessor matches a code of the ISO and IANA sets ignoring
  ASCII letter case, as those registers do, and a code of an openEHR-issued set exactly. The
  generator refuses an ISO or IANA set that lists two codes differing only in letter case.

It lives in-tree rather than behind an upstream URL for the same three reasons the
[BMM pins](../bmm/README.md) do:

1. **Network determinism.** The generated accessor must be a deterministic function of its
   input. Fetching at build time adds flaky builds and trust-on-first-use (TOFU) risk.
2. **Auditability.** A terminology bump is a real commit, reviewable in the PR diff.
3. **Air-gapped builds.** Consumers building behind a corporate proxy or in an air-gapped
   CI need the inputs in-tree.

## Consumers

| Consumer | Relationship |
|---|---|
| `openehr/terminology` | The generated, stdlib-only accessor. It exposes every group and code set of this pin as typed, closed, source-ordered values: groups with code → rubric, rubric → code, and membership lookups; code sets with membership lookups and the issuer and external id the pin gives each; plus the pinned release version (§ REQ-034). |
| `cmd/termgen` | The generator: reads both files of this pin, emits those tables. `make termgen` regenerates them; `make termgen-verify` fails the gate when they drift from either file (the [REQ-042](../../docs/specifications/bmm-conformance.md#req-042--generated-code-drift-detected) rule). |

The generator and the accessor land with the rest of the REQ-034 work; this pin is their
only input. Nothing else in the SDK parses these XML files at build or run time.

## Updating the pin

```bash
TERMINOLOGY_REF=Release-X.Y.Z make terminology-sync   # pin a named TERM release
make terminology-check                                # integrity + "is there a newer release?"
```

`terminology-sync` rewrites both XML files and `MANIFEST.txt`, then runs `make termgen` so
the generated tables follow the pin. It downloads both files before it replaces either, so
a failed fetch leaves the old pair in place. A bump is one explicit, reviewable commit that
moves both files together:

1. Run the sync at the new release tag.
2. Review the diffs: both XML files and the regenerated tables. A code or rubric that
   changed meaning, or a group or code set that lost a member, is a behaviour change; treat
   it as one.
3. Update the pin table in this README (release tag, byte sizes, root attributes, counts) and
   the release tag in the [`resources/README.md`](../README.md) inventory row. The manifest
   cannot update either of them.
4. Propose the one-line CHANGELOG bullet in the PR body; the CHANGELOG follows the rule in [`AGENTS.md § Code style and conventions`](../../AGENTS.md#code-style-and-conventions).
5. Commit the pin, the manifest, the regenerated tables and those doc rows together.

With no `TERMINOLOGY_REF`, `sync` re-fetches the ref already pinned in `MANIFEST.txt`
(and, with nothing pinned, the repository's latest GitHub release).

## Integrity

```bash
make terminology-verify   # offline sha256 against MANIFEST.txt; run by `make ci`
```

`terminology-verify` needs no network and no `curl`/`jq`, only `sha256sum` or `shasum`,
so it is safe inside the gate. It checks both files. It reports `FAILED … (sha256 mismatch)`
on any edit to a vendored file, `MISSING` when a file is gone, `NOT PINNED` when the manifest
has no hash line for one of the two files, and `UNTRACKED` for an XML in this directory that
the manifest does not list. **Never hand-edit the pin.** Its value lies in being byte-identical to
upstream ([AGENTS.md](../../AGENTS.md) § Gotchas); re-sync instead.

## References

- Normative contract: [`rm-modeling.md § openEHR terminology vocabulary (REQ-034)`](../../docs/specifications/rm-modeling.md#openehr-terminology-vocabulary-req-034).
- Sync script: [`../../scripts/sync-terminology.sh`](../../scripts/sync-terminology.sh) (`sync` / `check` / `verify`).
- Upstream specification: openEHR *Terminology* (TERM) component, <https://specifications.openehr.org/releases/TERM/>.
