# Packaging and module identity

**Status:** Draft

Normative requirements REQ-001 through REQ-005 for the Go module `github.com/cadasto/openehr-sdk-go`.

---

## REQ-001 — Module path

The SDK **MUST** be published as the Go module `github.com/cadasto/openehr-sdk-go`.

The path is all lowercase, matching idiomatic Go module naming and the GitHub organisation login. Consumer imports **MUST** use this exact spelling.

- **Lives in:** [`go.mod`](../../go.mod), all package import paths
- **Related:** [module-layout.md § Module identity](module-layout.md#module-identity)

---

## REQ-002 — Go version

The SDK **MUST** set the `go.mod` `go` directive to the latest stable patch of the **current stable Go release line (N)**; the authoritative pinned value is the `go` directive in [`go.mod`](../../go.mod) itself. When a newer major Go release becomes stable, the minimum **MUST** be raised to the new line — a **minor** SemVer bump ([releases.md § Versioning](../releases.md#versioning)). The directive **MUST NOT** name a pre-release (rc/beta) toolchain.

*Informative: at the time of writing the pin is `go 1.26.4`.*

- **Lives in:** [`go.mod`](../../go.mod)
- **Related:** [module-layout.md § Versioning](module-layout.md#versioning), [releases.md § Versioning](../releases.md#versioning)

---

## REQ-003 — License

The SDK **MUST** be distributed under the MIT License.

- **Lives in:** [`LICENSE`](../../LICENSE)

---

## REQ-004 — Semantic versioning

The SDK **MUST** follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html). Major versions `v2` and beyond **MUST** use Go's semantic-import-versioning convention (`…/v2/`).

- **Related:** [module-layout.md § Versioning](module-layout.md#versioning)

---

## REQ-005 — Internal boundary

Anything under `internal/` **MUST** be considered outside the public API surface. Consumers **MUST NOT** import from it, and the SDK **MAY** change it without notice.

- **Lives in:** `internal/` (e.g. `internal/bmmgen/`, `internal/bmmdiff/`)
- **Related:** [module-layout.md § The `internal/` boundary](module-layout.md#the-internal-boundary)
