# Canonical XML cassettes

Vendored canonical-XML fixtures used by the [canxml codec](../../../openehr/serialize/canxml/) and the [serialize probes](../../probes/serialize/).

## Provenance

Hand-crafted RM-instance XML pinned to the openEHR XSD release listed in [`openehr/serialize/canxml/doc.go`](../../../openehr/serialize/canxml/doc.go).

Source-of-truth RM graphs live in [`../canonical_json/`](../canonical_json/); the XML files in this directory MUST encode the same logical content for the same `<name>.xml` ↔ `<name>.json` pair when both exist. Round-trip parity is enforced by the cross-format test under [`canxml/roundtrip_test.go`](../../../openehr/serialize/canxml/).

## Refresh procedure

XML fixtures are pinned with the BMM/XSD version bump in `resources/bmm/`. To refresh:

1. Update `openehr/serialize/canxml/doc.go` with the new release tag.
2. Re-run the cross-format test (`go test ./openehr/serialize/canxml/...`) — it will surface any divergence between the JSON source-of-truth graphs and the XML fixtures.
3. Update the affected `<name>.xml` files to the new canonical form.
4. Add a CHANGELOG entry under `### Changed`.

## Wire profile

See `specs/wire.md § Canonical XML` and `docs/plans/2026-05-15-canonical-xml-serialization.md` for the rules these fixtures encode (BMM-order children, `xsi:type` first attribute, snake_case element names, compact whitespace).
