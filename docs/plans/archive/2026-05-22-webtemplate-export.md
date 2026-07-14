# Plan — WebTemplate JSON export

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-05-22 (activated 2026-07-13 — direct-slice decision; landed 2026-07-14)
**Status:** Done
**Owner:** SDK maintainers
**Covers:** [REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (registry row in [REQ.md](../../specifications/REQ.md))
**Probes:** [PROBE-075](../../specifications/conformance.md#probe-075--webtemplate-structural-parity)
**Decisions:** [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md) — reference implementation & id-generation lock
**Implementation:** landed
**Depends on:** landed compiled-template foundation — `openehr/template/` (REQ-100) + public bridge `openehr/templatecompile/` (REQ-111) + REQ-103 primitive constraints.
**Defers:** the shared simplified-template model abstraction (extracted with REQ-053 when a second consumer exists — [simplified-formats umbrella](../2026-06-23-simplified-formats.md)); FLAT/STRUCTURED codecs (REQ-053); WebTemplate→OPT round-trip; REST serving / content negotiation; exotic datatype inputs; the Better camelCase `id` variant; **archetype-reuse-under-slot templates** (duplicate compiled AQL paths — e.g. `corona_anamnese` — which `templatecompile` rejects; relaxing the compiler is a possible REQ-100/111 follow-up per ADR-0014).

## Goal

Ship `openehr/template/webtemplate` — a public package that projects a compiled operational template (`*templatecompile.Compiled`, REQ-111) into **EHRbase `openEHR_SDK` v2.3** WebTemplate JSON, for form-renderers / data-entry UIs / the CDR. Deterministic camelCase output; reference-locked lower-snake `id` generation (ADR-0014); core-datatype `inputs` from REQ-103 primitive constraints; **structural parity** (not byte parity) with a vendored EHRbase reference fixture (PROBE-075). This is the **direct slice** — no shared-model abstraction yet.

## Definition of Ready

- [x] `**Covers:**` lists the REQ (REQ-106).
- [x] Canonical normative prose exists — [clinical-modeling.md § REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) + REQ.md registry row.
- [x] The irreversible fork (reference-impl + id-generation lock) has an ADR — [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md). **Flip its Status to `Accepted` at Task 1 start** (maintainer sign-off on the REQ-106 spec is the acceptance moment).
- [x] Phases list concrete tasks and name the verification command (`go test`, `make spec-check`, `make ci`).

## Global Constraints

Every task's requirements implicitly include these:

- **Building-block independence (REQ-013):** `openehr/template/webtemplate/` imports only `openehr/templatecompile`, `openehr/template`, `openehr/rm/rminfo`, and the standard library. It **MUST NOT** import `transport/`, `auth/`, `openehr/client/*`, or `openehr/serialize/`. A guard test enforces this.
- **No reflection (REQ-024):** datatype dispatch is an explicit type switch on the REQ-103 `constraints.PrimitiveConstraint` sealed interface — never `reflect`.
- **Determinism:** identical `*Compiled` ⇒ byte-identical JSON, across runs and patch releases. Fixed struct field order; `encoding/json` sorts map keys.
- **Go floor:** do **not** touch the `go.mod` `go` directive; it stays the repo's existing minor `.0` floor.
- **Citations:** production code and tests carry `// REQ-106` (and `// PROBE-075` for the parity harness) comments.
- **Formatting:** gofumpt + goimports run on save (repo hook); `make fmt` before commits.
- **Tests:** table-driven, real code (no mocks); parity/golden tests load fixtures rather than hard-coding reference values.

## Definition of Done

- [x] Code + tests land with `// REQ-106` / `// PROBE-075` citations; `openehr/template/webtemplate` exports `Build` / `Marshal` / the typed shapes.
- [x] [`traceability.yaml`](../../specifications/traceability.yaml) REQ-106 `tests:`/`packages:` populated; `implementation: planned` → `landed`; REQ.md **Impl.** column `planned` → `landed`; ADR-0014 Status `Accepted`.
- [x] PROBE-075 implemented — 104/104 structural + input-suffix/type parity; deviations recorded in the package's `deviations.md`.
- [x] `make spec-check` and `make ci` pass.
- [x] Plan archived under [`docs/plans/archive/`](./) + plans/README + roadmap updated.

## Implementation checklist

| Step | Status |
|---|---|
| ADR-0014 flipped to Accepted | ✅ |
| Package + typed shapes (`webtemplate.go`, `doc.go`) | ✅ |
| Reference fixture vendored (or deferral recorded) | ✅ constrain_test |
| Structural transform (`build.go`) | ✅ 104/104 |
| id generation (`id.go`) | ✅ |
| inputs mapping (`inputs.go`) | ✅ 104/104 |
| Deterministic `Marshal` + round-trip goldens | ✅ |
| PROBE-075 structural parity + deviations list | ✅ |
| traceability / REQ.md / ADR status updated | ✅ |
| `make spec-check` + `make ci` | ✅ |

---

## File Structure

- `openehr/template/webtemplate/doc.go` — package doc: what WebTemplate is, the v2.3 lock, the deviations pointer.
- `openehr/template/webtemplate/webtemplate.go` — public `WebTemplate`/`Node`/`Input`/`InputListItem`/`Validation` shapes + `Build`/`Marshal`/`Option`.
- `openehr/template/webtemplate/build.go` — `*Compiled` → `*WebTemplate` recursive walk.
- `openehr/template/webtemplate/id.go` — web-id sanitisation + sibling disambiguation.
- `openehr/template/webtemplate/inputs.go` — per-RM-datatype `inputs[]` mapping.
- `openehr/template/webtemplate/*_test.go` — unit + golden + parity tests.
- `openehr/template/webtemplate/testdata/webtemplate/*.json` — SDK-generated round-trip goldens.
- `testkit/cassettes/webtemplate/constrain_test.{opt,webtemplate.json}` — vendored EHRbase reference (Apache-2.0; provenance in `testkit/cassettes/THIRD_PARTY_LICENSES.md`).

## Phases

### Phase 1 — Package skeleton & typed shapes

#### Task 1: Scaffold the package, shapes, and independence guard

**Files:**
- Modify: `docs/adr/0014-webtemplate-reference-implementation-lock.md` (Status → Accepted)
- Create: `openehr/template/webtemplate/doc.go`
- Create: `openehr/template/webtemplate/webtemplate.go`
- Test: `openehr/template/webtemplate/independence_test.go`

**Interfaces:**
- Produces: `type WebTemplate struct`, `type Node struct`, `type Input struct`, `type InputListItem struct`, `type Validation struct`; `func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error)`; `func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error)`; `type Option func(*config)`.

- [ ] **Step 1: Flip ADR-0014 to Accepted.** In `docs/adr/0014-...md`, change the Status line to `**Status:** Accepted, 2026-07-13 — maintainer sign-off on the REQ-106 specification; implementation lands in this plan.` (No other ADR change.)

- [ ] **Step 2: Write the independence guard test (RED).**

```go
// openehr/template/webtemplate/independence_test.go
package webtemplate_test

// REQ-013 — building-block independence: webtemplate must not pull in
// transport/auth/client/serialize.
import (
	"go/build"
	"strings"
	"testing"
)

func TestBuildingBlockIndependence(t *testing.T) {
	pkg, err := build.Import("github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate", "", 0)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	forbidden := []string{"/transport", "/auth", "/openehr/client", "/openehr/serialize"}
	for _, imp := range pkg.Imports {
		for _, f := range forbidden {
			if strings.Contains(imp, f) {
				t.Errorf("forbidden import %q (matches %q)", imp, f)
			}
		}
	}
}
```

- [ ] **Step 3: Run it — expect FAIL (no Go files in package yet).**

Run: `go test ./openehr/template/webtemplate/ -run TestBuildingBlockIndependence`
Expected: FAIL — `no buildable Go source files` / cannot import.

- [ ] **Step 4: Write `doc.go` and `webtemplate.go`.**

```go
// openehr/template/webtemplate/doc.go
// Package webtemplate exports a compiled openEHR operational template as
// EHRbase openEHR_SDK v2.3 "WebTemplate" JSON — a lossy, UI-oriented
// projection consumed by form renderers and data-entry clients.
//
// The shape and the consumer-critical "id" generation mirror EHRbase
// v2.3 (REQ-106, ADR-0014). Parity with the reference is structural, not
// byte-exact; accepted differences are listed in the package tests'
// deviations table.
package webtemplate
```

```go
// openehr/template/webtemplate/webtemplate.go
package webtemplate

import (
	"encoding/json"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// defaultVersion is the EHRbase openEHR_SDK WebTemplate schema version
// this package mirrors (REQ-106, ADR-0014).
const defaultVersion = "2.3"

// WebTemplate is the root of the exported document (REQ-106).
type WebTemplate struct {
	TemplateID      string   `json:"templateId"`
	Version         string   `json:"version"`
	DefaultLanguage string   `json:"defaultLanguage"`
	Languages       []string `json:"languages"`
	Tree            *Node    `json:"tree"`
}

// Node is one element of the WebTemplate tree (REQ-106).
type Node struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name,omitempty"`
	LocalizedName         string            `json:"localizedName,omitempty"`
	RMType                string            `json:"rmType"`
	NodeID                string            `json:"nodeId,omitempty"`
	Min                   int               `json:"min"`
	Max                   int               `json:"max"` // -1 = unbounded
	LocalizedNames        map[string]string `json:"localizedNames,omitempty"`
	LocalizedDescriptions map[string]string `json:"localizedDescriptions,omitempty"`
	AQLPath               string            `json:"aqlPath"`
	Inputs                []Input           `json:"inputs,omitempty"`
	Children              []*Node           `json:"children,omitempty"`
}

// Input is one logical form input under a leaf Node (REQ-106).
type Input struct {
	Suffix      string          `json:"suffix,omitempty"`
	Type        string          `json:"type"`
	List        []InputListItem `json:"list,omitempty"`
	ListOpen    bool            `json:"listOpen,omitempty"`
	Validation  *Validation     `json:"validation,omitempty"`
	Terminology string          `json:"terminology,omitempty"`
}

// InputListItem is one entry of a coded/ordinal input's list.
type InputListItem struct {
	Value        string            `json:"value"`
	Label        string            `json:"label,omitempty"`
	Ordinal      *int              `json:"ordinal,omitempty"`
	LocalizedLabels map[string]string `json:"localizedLabels,omitempty"`
}

// Validation carries a numeric/temporal constraint on an input.
type Validation struct {
	Range     *Range `json:"range,omitempty"`
	Precision *Range `json:"precision,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// Range is an inclusive/exclusive numeric interval.
type Range struct {
	Min          *float64 `json:"min,omitempty"`
	MinOp        string   `json:"minOp,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	MaxOp        string   `json:"maxOp,omitempty"`
}

type config struct {
	version         string
	defaultLanguage string
	languages       []string
}

// Option customises Build/Marshal.
type Option func(*config)

// WithVersion overrides the emitted schema version (default "2.3").
func WithVersion(v string) Option { return func(c *config) { c.version = v } }

// WithDefaultLanguage overrides the default language code.
func WithDefaultLanguage(code string) Option { return func(c *config) { c.defaultLanguage = code } }

// ErrEmptyTemplate is returned when the compiled template has no root.
var ErrEmptyTemplate = errors.New("webtemplate: compiled template has no root")

// Marshal builds and JSON-encodes the WebTemplate (REQ-106).
func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error) {
	wt, err := Build(c, opts...)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wt)
}
```

- [ ] **Step 5: Add a temporary `Build` stub so the package compiles.** In `webtemplate.go`:

```go
// Build is implemented in build.go (Task 3). Temporary stub for Task 1.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error) {
	return nil, ErrEmptyTemplate
}
```

- [ ] **Step 6: Run the guard test — expect PASS.**

Run: `go test ./openehr/template/webtemplate/ -run TestBuildingBlockIndependence`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
make fmt
git add docs/adr/0014-webtemplate-reference-implementation-lock.md openehr/template/webtemplate/
git commit -m "feat(webtemplate): scaffold package + WebTemplate/Node/Input shapes (REQ-106)"
```

### Phase 2 — Reference fixture (parity anchor)

#### Task 2: Vendor the EHRbase reference fixture

Parity-anchored (ADR-0014): the reference is the oracle. **If the fetch is blocked, record the deferral (Step 4b) and continue — later parity assertions fall back to SDK goldens.**

**Files:**
- Create: `testkit/cassettes/webtemplate/constrain_test.opt`
- Create: `testkit/cassettes/webtemplate/constrain_test.webtemplate.json`
- Modify: `testkit/cassettes/THIRD_PARTY_LICENSES.md`
- Test: `openehr/template/webtemplate/parity_test.go` (loader only in this task)

- [ ] **Step 1: Fetch the pinned EHRbase artefacts.** From the `openehr-kb` note's pinned commit `22b01e0c99b53669394e56da29c2410838b5cf7e`:

```bash
BASE=https://raw.githubusercontent.com/ehrbase/openEHR_SDK/22b01e0c99b53669394e56da29c2410838b5cf7e/test-data/src/main/resources
curl -fsSL "$BASE/operationaltemplate/constrain_test.opt" -o testkit/cassettes/webtemplate/constrain_test.opt
curl -fsSL "$BASE/webtemplate/constrain_test.json"        -o testkit/cassettes/webtemplate/constrain_test.webtemplate.json
```

- [ ] **Step 2: Record provenance + license.** Append to `testkit/cassettes/THIRD_PARTY_LICENSES.md`:

```markdown
## webtemplate/constrain_test.{opt,webtemplate.json}

Source: ehrbase/openEHR_SDK @ 22b01e0c99b53669394e56da29c2410838b5cf7e
  test-data/src/main/resources/{operationaltemplate,webtemplate}/constrain_test
License: Apache-2.0 (© EHRbase authors). Vendored unmodified as the REQ-106 / PROBE-075
WebTemplate structural-parity oracle. Not distributed as part of the SDK's runtime.
```

- [ ] **Step 3: Write the fixture-loader test (RED → GREEN).**

```go
// openehr/template/webtemplate/parity_test.go
package webtemplate_test

// PROBE-075 — the vendored EHRbase reference is the parity oracle.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const referenceDir = "../../../testkit/cassettes/webtemplate"

func loadReference(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(referenceDir, "constrain_test.webtemplate.json"))
	if err != nil {
		t.Skipf("reference fixture absent (PROBE-075 deferred, ADR-0014): %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("reference is not valid JSON: %v", err)
	}
	return m
}

func TestReferenceFixtureLoads(t *testing.T) {
	ref := loadReference(t)
	if _, ok := ref["tree"]; !ok {
		t.Fatalf("reference has no tree; keys=%v", keys(ref))
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4a (fetch succeeded): Run the loader test — expect PASS.**

Run: `go test ./openehr/template/webtemplate/ -run TestReferenceFixtureLoads -v`
Expected: PASS.

- [ ] **Step 4b (fetch blocked): record the deferral.** Skip the file creation; the test above `t.Skip`s automatically. Note in the commit body and in PROBE-075's Status that the fixture is pending; the plan continues on SDK goldens (Task 8) as the regression anchor.

- [ ] **Step 5: Commit.**

```bash
git add testkit/cassettes/webtemplate/ testkit/cassettes/THIRD_PARTY_LICENSES.md openehr/template/webtemplate/parity_test.go
git commit -m "test(webtemplate): vendor EHRbase constrain_test parity fixture (PROBE-075)"
```

### Phase 3 — Structural transform

#### Task 3: Build the tree (rmType / nodeId / aqlPath / min / max)

> **Corrected transform model (empirically derived from the `constrain_test` reference).** EHRbase's WebTemplate tree is **not** a structural mirror. `build.go` MUST implement, per [clinical-modeling.md § REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) "Output shape": (a) **keep** COMPOSITION / ENTRY types / EVENT / INTERVAL_EVENT / EVENT_CONTEXT / CLUSTER; (b) **collapse** each ELEMENT into a value leaf (`rmType`=value type, `nodeId`=element at-code, `aqlPath`=element path + `/value`); (c) **drop** HISTORY / ITEM_TREE / ITEM_LIST / ITEM_STRUCTURE as nodes, **folding their `attr[predicate]` into the descendants' `aqlPath`**; (d) emit data-bearing RM attributes (context `start_time`/`setting`, event `time`, entry `language`/`encoding`/`subject`, interval_event `math_function`/`width`) as leaves with `id`=attribute name. The reference `aqlPath` carries every archetype-id/at-code predicate and **differs from our compiled `AQLPath()`** — reconstruct it during the walk. The naive `buildNode` snippet below is superseded by this model; drive the implementation against the vendored reference (parity test) rather than the illustrative code.

**Files:**
- Create: `openehr/template/webtemplate/build.go`
- Test: `openehr/template/webtemplate/build_test.go`

**Interfaces:**
- Consumes: `templatecompile.Compile`, `CompiledNode.{RMTypeName,NodeID,ArchetypeID,AQLPath,Occurrences,Attributes,IsSlot}`, `CompiledAttribute.Children`, `template.Multiplicity.{Lower,Upper,UpperUnbounded}`.
- Produces: `func Build(...)` (replaces the Task 1 stub) building the full `*Node` tree; `func buildNode(n *templatecompile.CompiledNode, cfg *config) *Node`.

- [ ] **Step 1: Write the tree-shape test (RED).** Uses an existing repo OPT fixture (pick a small one; `openehr/template/testdata` has them).

```go
// openehr/template/webtemplate/build_test.go
package webtemplate_test

// REQ-106 — structural mirror of the compiled OPT (structural RM nodes retained).
import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

func compileFixture(t *testing.T, path string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

func TestBuildRootShape(t *testing.T) {
	c := compileFixture(t, "<PICK: an existing OPT under openehr/template/testdata>")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if wt.Version != "2.3" {
		t.Errorf("version = %q, want 2.3", wt.Version)
	}
	if wt.Tree == nil || wt.Tree.RMType != "COMPOSITION" {
		t.Fatalf("root rmType = %v, want COMPOSITION", wt.Tree)
	}
	if wt.Tree.AQLPath != "" {
		t.Errorf("root aqlPath = %q, want empty", wt.Tree.AQLPath)
	}
	// Structural node retained: a COMPOSITION has content children.
	if len(wt.Tree.Children) == 0 {
		t.Error("root has no children — structural nodes should be retained")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL (`ErrEmptyTemplate` stub).**

Run: `go test ./openehr/template/webtemplate/ -run TestBuildRootShape`
Expected: FAIL.

- [ ] **Step 3: Implement `build.go`.**

```go
// openehr/template/webtemplate/build.go
package webtemplate

import "github.com/cadasto/openehr-sdk-go/openehr/templatecompile"

// Build projects a compiled OPT into the typed WebTemplate tree (REQ-106).
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error) {
	if c == nil || c.Root() == nil {
		return nil, ErrEmptyTemplate
	}
	cfg := &config{version: defaultVersion}
	for _, o := range opts {
		o(cfg)
	}
	root := c.Root()
	wt := &WebTemplate{
		TemplateID: c.TemplateID(), // see Step 4 if the accessor name differs
		Version:    cfg.version,
		Tree:       buildNode(root, cfg),
	}
	// defaultLanguage/languages wired in Task 4.
	return wt, nil
}

func buildNode(n *templatecompile.CompiledNode, cfg *config) *Node {
	node := &Node{
		RMType:  n.RMTypeName(),
		NodeID:  n.NodeID(),
		AQLPath: n.AQLPath(),
	}
	if occ := n.Occurrences(); occ != nil {
		node.Min = occ.Lower()
		if occ.UpperUnbounded() {
			node.Max = -1
		} else {
			node.Max = occ.Upper()
		}
	}
	for _, attr := range n.Attributes() {
		for _, child := range attr.Children() {
			node.Children = append(node.Children, buildNode(child, cfg))
		}
	}
	return node
}
```

- [ ] **Step 4: Resolve the template-id accessor.** `Build` needs the template id. Confirm the accessor on `*Compiled` (Run: `go doc ./openehr/templatecompile Compiled`). If it is not `TemplateID()`, source the id from the parsed OPT instead: change `Build` to also accept it via `config`/`Option`, or read it from `template.OperationalTemplate` before compiling and pass through. Pick the smallest change that compiles; adjust the test's expectation if the field is exposed differently.

- [ ] **Step 5: Run the test — expect PASS.**

Run: `go test ./openehr/template/webtemplate/ -run TestBuildRootShape`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
make fmt
git add openehr/template/webtemplate/build.go openehr/template/webtemplate/build_test.go
git commit -m "feat(webtemplate): structural tree transform from compiled OPT (REQ-106)"
```

#### Task 4: Names, localized text, and languages

**Files:**
- Modify: `openehr/template/webtemplate/build.go`
- Test: `openehr/template/webtemplate/build_test.go`

**Interfaces:**
- Consumes: `CompiledNode.Term(code, lang string) (template.ArchetypeTerm, bool)` where `ArchetypeTerm.Items["text"]` is the display name and `Items["description"]` the description; `CompiledNode.NodeID()` supplies the at-code for `Term`.
- Produces: populated `Node.Name`/`LocalizedName`/`LocalizedNames`/`LocalizedDescriptions` and `WebTemplate.DefaultLanguage`/`Languages`.

- [ ] **Step 1: Write the name test (RED).**

```go
func TestNodeNamesAndLanguages(t *testing.T) {
	c := compileFixture(t, "<same fixture as TestBuildRootShape>")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if wt.DefaultLanguage == "" {
		t.Error("defaultLanguage empty")
	}
	if len(wt.Languages) == 0 {
		t.Error("languages empty")
	}
	// A named child carries a non-empty name.
	var named func(*webtemplate.Node) bool
	named = func(n *webtemplate.Node) bool {
		if n.Name != "" {
			return true
		}
		for _, ch := range n.Children {
			if named(ch) {
				return true
			}
		}
		return false
	}
	if !named(wt.Tree) {
		t.Error("no node carried a resolved name")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (`defaultLanguage empty`).

Run: `go test ./openehr/template/webtemplate/ -run TestNodeNamesAndLanguages`
Expected: FAIL.

- [ ] **Step 3: Implement name/language resolution in `build.go`.** Add to `buildNode` (after the occurrences block), resolving the term at the node's default language; add `defaultLang`/`langs` to `config`, defaulting from the OPT's languages (source them in `Build` — the OPT metadata carries language set; confirm the accessor via `go doc ./openehr/template`). Populate `Node.Name = term.Items["text"]` for the default language and `LocalizedNames[lang]`/`LocalizedDescriptions[lang]` across `cfg.languages`. Set `wt.DefaultLanguage`/`wt.Languages`.

```go
// inside buildNode, after occurrences:
if term, ok := n.Term(n.NodeID(), cfg.defaultLanguage); ok {
	node.Name = term.Items["text"]
	node.LocalizedName = node.Name
}
for _, lang := range cfg.languages {
	term, ok := n.Term(n.NodeID(), lang)
	if !ok {
		continue
	}
	if t := term.Items["text"]; t != "" {
		if node.LocalizedNames == nil {
			node.LocalizedNames = map[string]string{}
		}
		node.LocalizedNames[lang] = t
	}
	if d := term.Items["description"]; d != "" {
		if node.LocalizedDescriptions == nil {
			node.LocalizedDescriptions = map[string]string{}
		}
		node.LocalizedDescriptions[lang] = d
	}
}
```

- [ ] **Step 4: Run the test — expect PASS.**

Run: `go test ./openehr/template/webtemplate/ -run TestNodeNamesAndLanguages`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
make fmt
git add openehr/template/webtemplate/build.go openehr/template/webtemplate/build_test.go
git commit -m "feat(webtemplate): resolve node names + languages from terms (REQ-106)"
```

### Phase 4 — id generation (parity-critical)

#### Task 5: id sanitisation

**Files:**
- Create: `openehr/template/webtemplate/id.go`
- Test: `openehr/template/webtemplate/id_test.go`

**Interfaces:**
- Produces: `func webID(name, rmType string) string` (single-node sanitisation; disambiguation added in Task 6).

- [ ] **Step 1: Write the sanitisation table test (RED).** Cases are the confident rules (lower-snake, trim, collapse, simple diacritics, RM-type fallback); the uncertain rules (ß, leading digit, sibling collision) are pinned against the reference in Task 6.

```go
// openehr/template/webtemplate/id_test.go
package webtemplate

// REQ-106, ADR-0014 — lower-snake web-id sanitisation.
import "testing"

func TestWebIDSanitisation(t *testing.T) {
	cases := []struct{ name, rmType, want string }{
		{"Blood pressure", "OBSERVATION", "blood_pressure"},
		{"Systolic", "ELEMENT", "systolic"},
		{"  Diagnosis  ", "ELEMENT", "diagnosis"},
		{"Heart rate / pulse", "ELEMENT", "heart_rate_pulse"},
		{"Café", "ELEMENT", "cafe"},
		{"", "CLUSTER", "cluster"}, // RM-type fallback, lower-cased
	}
	for _, tc := range cases {
		if got := webID(tc.name, tc.rmType); got != tc.want {
			t.Errorf("webID(%q,%q) = %q, want %q", tc.name, tc.rmType, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run it — expect FAIL (`webID` undefined).**

Run: `go test ./openehr/template/webtemplate/ -run TestWebIDSanitisation`
Expected: FAIL — build error, undefined `webID`.

- [ ] **Step 3: Implement `id.go`.**

```go
// openehr/template/webtemplate/id.go
package webtemplate

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"          // confirm availability; else hand-roll diacritic strip
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// webID sanitises a node's display name into the EHRbase lower-snake web
// id, falling back to the RM type when the name is empty (REQ-106, ADR-0014).
func webID(name, rmType string) string {
	src := name
	if strings.TrimSpace(src) == "" {
		src = rmType
	}
	src = stripDiacritics(src)
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(src) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func stripDiacritics(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return out
}
```

**Note on `golang.org/x/text`:** confirm it is already an allowed dependency (Run: `grep golang.org/x/text go.mod`). If it is **not** present and REQ-013/dependency policy forbids adding it, replace `stripDiacritics` with a hand-rolled NFD-free ASCII fold over the Latin-1 diacritic range — do not add a new module dependency without an ADR.

- [ ] **Step 4: Run the test — expect PASS.**

Run: `go test ./openehr/template/webtemplate/ -run TestWebIDSanitisation`
Expected: PASS.

- [ ] **Step 5: Wire `webID` into `buildNode`** (set `node.ID = webID(node.Name, node.RMType)` after names resolve) and re-run `TestBuildRootShape`/`TestNodeNamesAndLanguages` — expect PASS. Commit.

```bash
make fmt
git add openehr/template/webtemplate/id.go openehr/template/webtemplate/id_test.go openehr/template/webtemplate/build.go
git commit -m "feat(webtemplate): lower-snake web-id sanitisation (REQ-106, ADR-0014)"
```

#### Task 6: Sibling disambiguation, derived from the reference

**Files:**
- Modify: `openehr/template/webtemplate/id.go`, `openehr/template/webtemplate/build.go`
- Test: `openehr/template/webtemplate/parity_test.go`

**Interfaces:**
- Produces: disambiguated `Node.ID` — unique among siblings, matching the reference rule.

- [ ] **Step 1: Write the id-parity test (RED/skip-aware).** It loads the reference and asserts our id at each `aqlPath` equals the reference id — no hard-coded ids.

```go
func TestIDParityAgainstReference(t *testing.T) {
	ref := loadReference(t) // t.Skip if fixture absent
	refByPath := indexByAQLPath(ref) // map[aqlPath]id — walk ref["tree"]
	c := compileFixture(t, referenceDir+"/constrain_test.opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var mismatches []string
	walk(wt.Tree, func(n *webtemplate.Node) {
		want, ok := refByPath[n.AQLPath]
		if ok && want != n.ID {
			mismatches = append(mismatches, n.AQLPath+": got "+n.ID+" want "+want)
		}
	})
	if len(mismatches) > 0 {
		t.Errorf("id mismatches vs reference (%d):\n%s", len(mismatches), strings.Join(mismatches, "\n"))
	}
}
```

(Add the `indexByAQLPath`/`walk` helpers in the test file: recursive descent over the reference `map[string]any` tree and over `*webtemplate.Node`.)

- [ ] **Step 2: Run it — expect FAIL (collisions produce duplicate ids) or SKIP (fixture absent).**

Run: `go test ./openehr/template/webtemplate/ -run TestIDParityAgainstReference -v`
Expected: FAIL listing mismatches (or SKIP — then implement the rule from the openehr-kb note and defer the assertion).

- [ ] **Step 3: Implement sibling disambiguation.** In `buildNode`, after building a node's children, assign ids in a second pass that tracks seen ids among siblings and applies the reference's collision suffix (observe the exact suffix in the mismatch output / reference JSON — e.g. a trailing `_<n>` or a parent-qualified form — and encode it). Keep it a pure function of the sibling set so output stays deterministic.

- [ ] **Step 4: Run the test — expect PASS (or documented SKIP).** Commit.

```bash
make fmt
git add openehr/template/webtemplate/id.go openehr/template/webtemplate/build.go openehr/template/webtemplate/parity_test.go
git commit -m "feat(webtemplate): sibling id disambiguation matching EHRbase reference (REQ-106, PROBE-075)"
```

### Phase 5 — inputs

#### Task 7: Core-datatype inputs mapping

**Files:**
- Create: `openehr/template/webtemplate/inputs.go`
- Modify: `openehr/template/webtemplate/build.go` (call `inputsFor` at leaves)
- Test: `openehr/template/webtemplate/inputs_test.go`

**Interfaces:**
- Consumes: `CompiledNode.PrimitiveConstraint() constraints.PrimitiveConstraint` (REQ-103 sealed interface) + the node's RM type.
- Produces: `func inputsFor(n *templatecompile.CompiledNode) []Input`.

- [ ] **Step 1: Write the inputs table test (RED)** covering each core datatype from [clinical-modeling.md § REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export): DV_TEXT, DV_CODED_TEXT, DV_QUANTITY, DV_COUNT, DV_ORDINAL, DV_DATE_TIME/DATE/TIME, DV_BOOLEAN, DV_PROPORTION — asserting `suffix`/`type` (and `list`/`listOpen` where relevant). Use OPT fixtures that exercise each type, or build minimal `CompiledNode`s via `templatecompile.Compile` on a fixture that contains them (prefer the vendored `constrain_test` fixture, which covers DV_TEXT / CODED_TEXT / QUANTITY / COUNT / ORDINAL / DATE_TIME / DURATION / PROPORTION). Assert the exotic case (e.g. DV_MULTIMEDIA if present) yields a node with `len(Inputs)==0` and no error.

- [ ] **Step 2: Run it — expect FAIL (`inputsFor` undefined).**

- [ ] **Step 3: Implement `inputs.go`** as an explicit type switch on the REQ-103 `constraints.PrimitiveConstraint` concrete types (no reflection) → `[]Input`, per the REQ-106 table. Confirm the concrete constraint type names via `go doc ./openehr/template/constraints`. Exotic/unmapped constraints return `nil` (node emitted without inputs). Call `node.Inputs = inputsFor(n)` in `buildNode` at leaf ELEMENT nodes.

- [ ] **Step 4: Run the test — expect PASS.** Commit.

```bash
make fmt
git add openehr/template/webtemplate/inputs.go openehr/template/webtemplate/inputs_test.go openehr/template/webtemplate/build.go
git commit -m "feat(webtemplate): core-datatype inputs mapping (REQ-106, REQ-103)"
```

### Phase 6 — serialization & determinism

#### Task 8: Deterministic Marshal + round-trip goldens

**Files:**
- Test: `openehr/template/webtemplate/golden_test.go`
- Create: `openehr/template/webtemplate/testdata/webtemplate/*.json`

- [ ] **Step 1: Write the determinism + golden test (RED).** For a small set of existing OPT fixtures, `Marshal` twice and assert byte-equality (determinism); then compare against a checked-in golden with a `-update` flag idiom.

```go
// openehr/template/webtemplate/golden_test.go
package webtemplate_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

var update = flag.Bool("update", false, "update golden files")

func TestMarshalDeterministicAndGolden(t *testing.T) {
	fixtures := []string{"<opt-1>", "<opt-2>"} // existing OPTs under openehr/template/testdata
	for _, f := range fixtures {
		c := compileFixture(t, f)
		a, err := webtemplate.Marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b, _ := webtemplate.Marshal(c)
		if string(a) != string(b) {
			t.Fatalf("%s: non-deterministic output", f)
		}
		golden := filepath.Join("testdata/webtemplate", filepath.Base(f)+".json")
		if *update {
			os.WriteFile(golden, a, 0o644)
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("read golden (run -update): %v", err)
		}
		if string(a) != string(want) {
			t.Errorf("%s: output != golden", f)
		}
	}
}
```

- [ ] **Step 2: Run without goldens — expect FAIL.** Run: `go test ./openehr/template/webtemplate/ -run TestMarshalDeterministicAndGolden`. Expected: FAIL (missing golden).

- [ ] **Step 3: Generate goldens.** Run: `go test ./openehr/template/webtemplate/ -run TestMarshalDeterministicAndGolden -update`. Inspect the JSON for sanity (shape, ids, inputs).

- [ ] **Step 4: Run again — expect PASS.** Commit goldens + test.

```bash
make fmt
git add openehr/template/webtemplate/golden_test.go openehr/template/webtemplate/testdata/
git commit -m "test(webtemplate): deterministic Marshal + round-trip goldens (REQ-106)"
```

### Phase 7 — conformance

#### Task 9: PROBE-075 structural parity + deviations list

**Files:**
- Modify: `openehr/template/webtemplate/parity_test.go`
- Create: `openehr/template/webtemplate/deviations.md` (the documented-deviations list)

- [ ] **Step 1: Write the structural-parity test (RED/skip-aware).** Beyond ids (Task 6), assert per reference node at each `aqlPath`: `rmType`, `min`, `max`, and input `suffix`/`type` sets match. Collect every difference; a difference whose signature is on the deviations list is ignored, otherwise it fails.

- [ ] **Step 2: Run it — expect FAIL (real differences) or SKIP.** Triage each diff: a genuine bug → fix the transform; an accepted incidental (field ordering, optional-field absence, a known id edge) → add a line to `deviations.md` with its rationale.

- [ ] **Step 3: Iterate to green.** Loop Step 2 until parity holds modulo the documented deviations. Keep `deviations.md` authoritative — an unlisted diff must fail.

- [ ] **Step 4: Commit.**

```bash
make fmt
git add openehr/template/webtemplate/parity_test.go openehr/template/webtemplate/deviations.md
git commit -m "test(webtemplate): PROBE-075 structural parity + documented deviations (REQ-106)"
```

### Phase 8 — close-out

#### Task 10: Traceability, statuses, gates

**Files:**
- Modify: `docs/specifications/traceability.yaml`, `docs/specifications/REQ.md`, `docs/specifications/conformance.md`, `docs/roadmap.md`, `docs/plans/README.md`

- [ ] **Step 1: Populate REQ-106 traceability.** Add the `tests:` list (every new `*_test.go`) to the REQ-106 entry; flip `implementation: planned` → `landed`.
- [ ] **Step 2: Flip REQ.md Impl. column** REQ-106 `planned` → `landed`.
- [ ] **Step 3: Update PROBE-075 Status** in `conformance.md` to Implemented (or Deferred-to-goldens if the fixture was blocked), pointing at the parity test.
- [ ] **Step 4: Run `make spec-check`.** Expected: `spec-check: OK`.
- [ ] **Step 5: Run `make ci`.** Expected: all green (fmt, vet, spec-check, codegen drift, race tests).
- [ ] **Step 6: Update `docs/roadmap.md` + `docs/plans/README.md`** (WebTemplate export landed) and set this plan **Status: complete** (archive move handled by `sdd-archive` at finish).
- [ ] **Step 7: Commit.**

```bash
git add docs/
git commit -m "docs(webtemplate): land REQ-106/PROBE-075 traceability + statuses"
```

## Self-review (author)

- **Spec coverage:** REQ-106 §Surface → Task 1/3; §Output shape + determinism → Task 3/8; §id generation → Task 5/6; §inputs → Task 7; §Conformance/deviations → Task 9; §Building-block independence → Task 1 guard. ADR-0014 lock → Task 2 (fixture) + Task 5/6 (id) + Task 9 (structural-not-byte). PROBE-075 → Task 9. All covered.
- **Placeholders:** the `<PICK …>` / `<opt-1>` markers are deliberate fixture-selection points (the implementer picks a concrete existing OPT under `openehr/template/testdata`); the `TemplateID()` accessor and the `golang.org/x/text`/constraint-type names carry explicit confirm-steps because they are the only facts not verified at plan-authoring time. No behavioural TODOs.
- **Type consistency:** `Build`/`Marshal`/`buildNode`/`webID`/`inputsFor`/`Node`/`Input` names and signatures are consistent across Tasks 1–9.

## Mapping to specs

- [clinical-modeling.md § REQ-106](../../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — normative contract.
- [REQ.md](../../specifications/REQ.md) — registry row.
- [ADR-0014](../../adr/0014-webtemplate-reference-implementation-lock.md) — reference-impl & id-generation lock.
- [conformance.md § PROBE-075](../../specifications/conformance.md#probe-075--webtemplate-structural-parity) — structural-parity probe.
- [simplified-formats umbrella](../2026-06-23-simplified-formats.md) — where the shared model is extracted later (REQ-053).
