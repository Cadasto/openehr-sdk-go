package generic_test

// REQ-178 (docs/specifications/clinical-modeling.md § REQ-178): the generic
// EHR_STATUS and FOLDER operational templates the SDK ships, as documents
// and compiled.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template/generic"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// shipped is one template the package ships, with what the tests hold it to.
type shipped struct {
	file        string // the document's file in the package directory
	id          string // its template id, as a literal
	constant    string // the exported constant that names it
	rmType      string
	archetypeID string
	sha256      string // the pinned digest of the document
	document    func() []byte
	compiled    func() *templatecompile.Compiled
	// validate asserts the root's Go type and validates it against the
	// compiled template, returning the root's archetype node id too.
	validate func(t *testing.T, root any) (string, validation.Result)
}

var templates = []shipped{
	{
		file:        "generic_ehr_status.v1.opt",
		id:          "generic_ehr_status.v1",
		constant:    generic.EHRStatusTemplateID,
		rmType:      "EHR_STATUS",
		archetypeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		sha256:      "73072eee72ce161373dcb01ef1e39691a915d7c8af3c6766283be24c9c510026",
		document:    generic.EHRStatusOPT,
		compiled:    generic.EHRStatus,
		validate: func(t *testing.T, root any) (string, validation.Result) {
			t.Helper()
			status, ok := root.(*rm.EHRStatus)
			if !ok || status == nil {
				t.Fatalf("Generate returned %T, want a non-nil *rm.EHRStatus", root)
			}
			return status.ArchetypeNodeID, validation.ValidateEHRStatus(status, generic.EHRStatus())
		},
	},
	{
		file:        "generic_folder.v1.opt",
		id:          "generic_folder.v1",
		constant:    generic.FolderTemplateID,
		rmType:      "FOLDER",
		archetypeID: "openEHR-EHR-FOLDER.generic.v1",
		sha256:      "0712a7c6c59305759dacf6e336fdb1d715424c2286ee2fe518a5c62711643bd6",
		document:    generic.FolderOPT,
		compiled:    generic.Folder,
		validate: func(t *testing.T, root any) (string, validation.Result) {
			t.Helper()
			folder, ok := root.(*rm.Folder)
			if !ok || folder == nil {
				t.Fatalf("Generate returned %T, want a non-nil *rm.Folder", root)
			}
			return folder.ArchetypeNodeID, validation.ValidateFolder(folder, generic.Folder())
		},
	},
}

// TestREQ178CompiledOnceAndShared runs first in this package's external
// tests and not in parallel, so its goroutines make the first calls to
// EHRStatus and Folder when the test binary runs it first or alone.
func TestREQ178CompiledOnceAndShared(t *testing.T) {
	for _, tc := range templates {
		t.Run(tc.id, func(t *testing.T) {
			got := make([]*templatecompile.Compiled, 16)
			var wg sync.WaitGroup
			for i := range got {
				wg.Go(func() { got[i] = tc.compiled() })
			}
			wg.Wait()

			first := tc.compiled()
			if first == nil {
				t.Fatalf("compiled %s = nil, want the compiled template", tc.id)
			}
			if again := tc.compiled(); again != first {
				t.Errorf("two calls for %s returned %p and %p, want the same pointer", tc.id, first, again)
			}
			for i, c := range got {
				if c != first {
					t.Errorf("concurrent call %d for %s returned %p, want %p, the pointer every call returns", i, tc.id, c, first)
				}
			}
		})
	}
}

func TestREQ178DocumentIsTheEmbeddedFileInANewSlice(t *testing.T) {
	t.Parallel()
	for _, tc := range templates {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			first, second := tc.document(), tc.document()
			if !bytes.Equal(first, want) {
				t.Fatalf("document of %s differs from %s (%d bytes, want %d)", tc.id, tc.file, len(first), len(want))
			}

			clear(first)
			if !bytes.Equal(second, want) {
				t.Errorf("clearing one result for %s changed an earlier result: the calls share their bytes", tc.id)
			}
			if next := tc.document(); !bytes.Equal(next, want) {
				t.Errorf("clearing one result for %s changed the next call's result: the calls share their bytes", tc.id)
			}
		})
	}
}

func TestREQ178DocumentReadsAndCompilesStrictly(t *testing.T) {
	t.Parallel()
	for _, tc := range templates {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			if tc.constant != tc.id {
				t.Errorf("template id constant = %q, want %q", tc.constant, tc.id)
			}

			operationalTemplate, err := aom14.ReadOperationalTemplate(bytes.NewReader(tc.document()), canxml.WithStrict())
			if err != nil {
				t.Fatalf("ReadOperationalTemplate(%s, WithStrict) = %v, want no error", tc.file, err)
			}
			c, err := templatecompile.Compile(operationalTemplate, templatecompile.WithStrict())
			if err != nil {
				t.Fatalf("Compile(%s, WithStrict) = %v, want no error", tc.file, err)
			}

			for name, got := range map[string]*templatecompile.Compiled{"strict compile": c, "package's compiled template": tc.compiled()} {
				if id := got.TemplateID(); id != tc.id {
					t.Errorf("%s of %s: TemplateID() = %q, want %q", name, tc.file, id, tc.id)
				}
				root := got.Root()
				if root == nil {
					t.Fatalf("%s of %s: Root() = nil", name, tc.file)
				}
				if rmType := root.RMTypeName(); rmType != tc.rmType {
					t.Errorf("%s of %s: root RM type = %q, want %q", name, tc.file, rmType, tc.rmType)
				}
				if archetypeID := root.ArchetypeID(); archetypeID != tc.archetypeID {
					t.Errorf("%s of %s: root archetype id = %q, want %q", name, tc.file, archetypeID, tc.archetypeID)
				}
			}
		})
	}
}

func TestREQ178GeneratedInstancesValidate(t *testing.T) {
	t.Parallel()
	policies := []instance.Policy{instance.Required, instance.Medium, instance.Large}
	fills := []instance.ValueFill{instance.ExampleFill, instance.RandomFill}
	now := time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

	for _, tc := range templates {
		for _, policy := range policies {
			for _, fill := range fills {
				name := fmt.Sprintf("%s/%s/%s", tc.id, policy, fill)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					root, err := instance.Generate(t.Context(), tc.compiled(), instance.Options{
						Policy:      policy,
						ValueFill:   fill,
						Now:         now,
						ValueSource: rand.NewPCG(178, 178),
					})
					if err != nil {
						t.Fatalf("Generate(%s) = %v, want no error", name, err)
					}
					nodeID, res := tc.validate(t, root)
					if nodeID != tc.archetypeID {
						t.Errorf("Generate(%s): root archetype_node_id = %q, want %q", name, nodeID, tc.archetypeID)
					}
					if !res.OK {
						t.Errorf("Generate(%s) then validation: OK = false, want true; issues: %+v", name, res.Issues)
					}
				})
			}
		}
	}
}

func TestREQ178DocumentDigestIsPinned(t *testing.T) {
	t.Parallel()
	for _, tc := range templates {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			sum := sha256.Sum256(tc.document())
			if got := hex.EncodeToString(sum[:]); got != tc.sha256 {
				t.Errorf("%s changed: its SHA-256 is %s, the pin is %s.\n"+
					"Template %s is stored on servers, with the instances written against it, so:\n"+
					"  - a change must not refuse an instance the template under the same id accepted before;\n"+
					"    a change that would ships under a new id (such as the next version) and the package keeps the old id's template and constant;\n"+
					"  - the CHANGELOG records the change, under the release that ships it.\n"+
					"When the change keeps both rules, move this pin to %s in the same change.",
					tc.file, got, tc.sha256, tc.id, got)
			}
		})
	}
}
