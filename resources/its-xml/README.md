# `resources/its-xml/`: pinned openEHR ITS-XML schemas

Vendored, version-pinned XML Schema (XSD) files from the openEHR ITS-XML specification. They are
the source of each class's `xs:sequence`: the order in which the canonical-XML encoders write a
class's child elements ([ADR 0002 D10](../../docs/adr/0002-bmm-codegen-decisions.md),
[REQ-056](../../docs/specifications/wire.md#req-056)). The BMM schemas under
[`resources/bmm/`](../bmm/README.md) give no such order, so the code generator reads it from here.

The generator reads only the element sequences. These schemas declare
`targetNamespace="http://schemas.openehr.org/v2"`, while canonical XML writes the
`http://schemas.openehr.org/v1` default namespace that REQ-056 requires.

## Provenance

- **Source:** [`openEHR/specifications-ITS-XML`](https://github.com/openEHR/specifications-ITS-XML).
- **Roots:** `components/RM/Release-1.1.0/*.xsd`, `components/BASE/Release-1.1.0/*.xsd`,
  `components/AM/Release-1.4/Archetype.xsd` and `components/AM/Release-1.4/OpenehrProfile.xsd`.
  A `*` matches within its own directory, so the `documents/` subdirectory of the RM release is not
  a root: its files declare top-level elements only (`composition`, `version`, `extract` and the
  like), over types the main files define.
- **Closure:** the sync script follows every `xs:include`, `xs:import` and `xs:redefine` from the
  roots, transitively, and vendors each file it reaches. Every file keeps its upstream path under
  this directory, so each `schemaLocation` still resolves here.
- **Pin + integrity:** the exact upstream commit and a per-file `sha256` are recorded in
  [`MANIFEST.txt`](MANIFEST.txt). Regenerate with `make its-xml-sync`; verify with
  `make its-xml-drift` (see [the sync script](../../scripts/sync-its-xml-schemas.sh)).

## Files

At the pinned commit the closure holds these files.

| File | Classes | Reached as |
|---|---|---|
| `components/RM/Release-1.1.0/DataTypes.xsd` | RM data types (`DV_*`, `CODE_PHRASE`, `TERM_MAPPING`, ...) | root |
| `components/RM/Release-1.1.0/DataStructures.xsd` | RM data structures (`ITEM_TREE`, `HISTORY`, `EVENT`, ...) | root |
| `components/RM/Release-1.1.0/Common.xsd` | RM common (`LOCATABLE`, `PARTY_PROXY`, `FEEDER_AUDIT`, `VERSION`, `FOLDER`, ...) | root |
| `components/RM/Release-1.1.0/Ehr.xsd` | RM EHR and composition (`EHR_STATUS`, `COMPOSITION`, `OBSERVATION`, ...) | root |
| `components/RM/Release-1.1.0/Demographic.xsd` | RM demographic (`PERSON`, `ORGANISATION`, `ROLE`, ...) | root |
| `components/RM/Release-1.1.0/EhrExtract.xsd` | RM EHR extract (`EXTRACT`, `X_VERSIONED_OBJECT`, ...) | root |
| `components/BASE/Release-1.1.0/BaseTypes.xsd` | BASE base types (`UID`, `OBJECT_ID`, `ARCHETYPE_ID`, ...) | root |
| `components/BASE/Release-1.1.0/Resource.xsd` | BASE resource (`AUTHORED_RESOURCE`, `RESOURCE_DESCRIPTION`, ...) | root |
| `components/AM/Release-1.4/Archetype.xsd` | AOM 1.4 (`ARCHETYPE`, `C_OBJECT`, `C_ATTRIBUTE`, ...) | root |
| `components/AM/Release-1.4/OpenehrProfile.xsd` | openEHR Archetype Profile 1.4 (`C_DV_QUANTITY`, `C_DV_ORDINAL`, ...) | root |
| `components/AM/Release-1.4/Resource.xsd` | AOM 1.4 resource (`AUTHORED_RESOURCE`, `RESOURCE_DESCRIPTION`, ...) | included by `Archetype.xsd` |
| `components/BASE/latest/BaseTypes.xsd` | BASE base types (`UID`, `OBJECT_ID`, `ARCHETYPE_ID`, ...) | included by RM `DataTypes.xsd` |

Two inclusions are worth knowing when reading the files:

- The RM Release-1.1.0 `DataTypes.xsd` includes `../../BASE/latest/BaseTypes.xsd`, not the BASE
  Release-1.1.0 copy. At the pinned commit the two `BaseTypes.xsd` files differ only in the schema's
  `version` attribute (`v1.2.0` against `v1.1.0`).
- The AM Release-1.4 `Resource.xsd` includes `../../RM/Release-1.1.0/Common.xsd`, so the AOM 1.4
  types resolve against the RM Release-1.1.0 schemas.

## Updating the pin

```bash
make its-xml-sync                              # fetch from master, rewrite MANIFEST.txt
ITS_XML_REF=Release-2.0.0v2 make its-xml-sync  # pin a tag / branch / commit instead
make its-xml-drift                             # verify local copies + report if upstream advanced
```

A version bump is an explicit, reviewable commit: diff the schemas and `MANIFEST.txt`. Never edit
a vendored schema by hand; re-sync instead. After a re-pin, re-run `make codegen`, so the generated
XML encoders follow the new sequences, and commit what it changes with the bump: `make codegen-check`
fails until the generated files match the schemas.

## Licence

The upstream repository is licensed under the **Apache License 2.0** (its `LICENSE` file). The
schemas carry no licence header of their own; the three AM Release-1.4 files carry the comment
"Authored by Ocean Informatics 2008.12.22". They are vendored unmodified. Repository-wide
inventory: [`LICENSING.md`](../../LICENSING.md).
