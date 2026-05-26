# testkit/cassettes

Vendored test cassettes used by the SDK's codec, validation, and probe
tests. Cassettes are checked into the repository so CI does **not**
require any sibling repo to be cloned (REQ-082).

## Layout

| Sub-directory | Format | Used by |
|---|---|---|
| `canonical_json/` | openEHR canonical JSON compositions | `openehr/serialize/canjson`, PROBE-030, PROBE-031 |

## Provenance

### `canonical_json/`

Vendored canonical JSON compositions from the reference CDR load harness (internal snapshot, May 2026). Upstream layout and refresh steps stay in the private consumer checkout — not documented here.

| File | Notes |
|---|---|
| `body_weight.json` | Minimal Composition with a single OBSERVATION. |
| `BMI.json` | Multi-event observation with HISTORY and ITEM_TREE protocol. |
| `clinical_note.json` | EVALUATION-bearing Composition. |
| `vital_signs.json` | OBSERVATION with multiple data points and reference ranges. |

## Conventions

- Cassettes are immutable inputs. **Never** hand-edit a vendored
  cassette to make a test pass — fix the codec, or open a follow-up to
  refresh the cassette from upstream.
- New cassette directories require a row in the Layout table and a
  Provenance subsection.
- Cassettes that exercise SDK-emitted bytes (e.g. round-trip outputs)
  live next to their test as `testdata/`, not here.
