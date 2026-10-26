---
kind: specification
---

# Packaging and module identity

**Status:** Draft

Normative requirements for the Go module `github.com/cadasto/openehr-sdk-go`: its path (REQ-001), its versioning (REQ-004) and its internal boundary (REQ-005). REQ-002 and REQ-003 are retired.

---

## REQ-001 — Module path

The SDK **MUST** be published as the Go module `github.com/cadasto/openehr-sdk-go`.

The path is all lowercase, matching idiomatic Go module naming and the GitHub organisation login, and consumer imports use this exact spelling.

The path is locked. Renaming it is a change [module-layout.md § Versioning](module-layout.md#versioning) maps to major, and a rename **MUST** follow a deprecation cycle of at least one minor release. A patch release **MUST NOT** change the module path. The `/vN` suffix that [REQ-004](#req-004--semantic-versioning) requires for major versions v2 and beyond is Go's semantic import version of this same path, not a rename, so the deprecation cycle does not apply to it.

- **Related:** [module-layout.md § Module identity](module-layout.md#module-identity)

---

## REQ-004 — Semantic versioning

The SDK **MUST** follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html). Major versions `v2` and beyond **MUST** use Go's semantic-import-versioning convention (`…/v2/`).

- **Related:** [module-layout.md § Versioning](module-layout.md#versioning)

---

## REQ-005 — Internal boundary

Anything under `internal/` is outside the public API surface: the Go toolchain refuses an import of it from another module, and the SDK **MAY** change it without notice.

- **Related:** [module-layout.md § The `internal/` boundary](module-layout.md#the-internal-boundary)
