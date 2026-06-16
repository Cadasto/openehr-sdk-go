# AQL grammar profile

The SDK parses AQL with an ANTLR grammar maintained here: the **official openEHR
grammar** plus a small, documented set of deltas (the *SDK grammar profile*,
REQ-109). See [ADR 0007](../../../docs/adr/0007-aql-antlr-grammar-profile.md) for
the strategy and [`../../../docs/plans/2026-06-15-aql-lint.md`](../../../docs/plans/2026-06-15-aql-lint.md)
for the full plan.

## Layout

| Path | Role |
|---|---|
| `baseline/` | Frozen copy of the official openEHR `AqlLexer.g4` / `AqlParser.g4` + `PIN` (provenance). **Read-only** — changes only on a QUERY release bump. |
| `active/` | What `make aqlgen` consumes: `baseline/` + the deltas in `DIVERGENCES.md`. |
| `DIVERGENCES.md` | One `SDK-AQL-NNN` row per delta between `baseline/` and `active/`. |

The generated Go parser is committed under
[`../../../openehr/aql/parse/gen/`](../../../openehr/aql/parse/gen/). `make ci`,
`go build`, and `go test` never run the generator — they compile the committed
parser against the pure-Go runtime.

## Regenerate (maintainer-only; needs Docker, not a host JRE)

```bash
make aqlgen          # regenerate parse/gen/ from active/
make aqlgen-verify   # fail if the committed parser drifts from active/
```

The generator is the ANTLR Java tool, confined to the `antlr` Docker stage; the
runtime (`github.com/antlr4-go/antlr/v4`) is pure Go. **Bump the tool and runtime
versions together** (see `baseline/PIN`) — they are released in lockstep.

## Rebasing onto a new QUERY release

1. Re-import the new official `.g4` into `baseline/`; update `baseline/PIN`.
2. For each `SDK-AQL-NNN`: re-apply to `active/`, or drop the row if upstream
   absorbed it.
3. `make aqlgen && make aqlgen-verify`; run `go test ./openehr/aql/...`.
4. Note the bump in `CHANGELOG.md` (artefact class: "AQL grammar profile").

## Licence

The openEHR grammars are **CC-BY-SA 4.0**, © openEHR Foundation. The SDK's deltas
in `active/` are a documented derivative work under the same terms. Attribution
is retained in the grammar file headers.
