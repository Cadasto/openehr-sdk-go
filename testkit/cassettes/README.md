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

Source: `openehr-cdr` (formerly `openehr-go-poc`) repository,
`cmd/benchmark/internal/fixtures/compositions/`.

| File | Notes |
|---|---|
| `body_weight.json` | Minimal Composition with a single OBSERVATION. |
| `BMI.json` | Multi-event observation with HISTORY and ITEM_TREE protocol. |
| `clinical_note.json` | EVALUATION-bearing Composition. |
| `vital_signs.json` | OBSERVATION with multiple data points and reference ranges. |

Source commit: `0781322c60d3ae2bfcd8f0863cfe0d69c5753a61` (May 2026).

### Refresh command

```sh
# from the SDK root
cp /src/cadasto/openehr-cdr/cmd/benchmark/internal/fixtures/compositions/{body_weight,BMI,clinical_note,vital_signs}.json \
  testkit/cassettes/canonical_json/
```

Update the source commit above after refreshing.

## Conventions

- Cassettes are immutable inputs. **Never** hand-edit a vendored
  cassette to make a test pass — fix the codec, or open a follow-up to
  refresh the cassette from upstream.
- New cassette directories require a row in the Layout table and a
  Provenance subsection.
- Cassettes that exercise SDK-emitted bytes (e.g. round-trip outputs)
  live next to their test as `testdata/`, not here.
