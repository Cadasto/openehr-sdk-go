package generic

import (
	"bytes"
	_ "embed" // for the go:embed directives below
	"fmt"
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// The template ids of the two templates the package ships.
const (
	// EHRStatusTemplateID is the template id of the generic EHR_STATUS
	// template.
	EHRStatusTemplateID = "generic_ehr_status.v1"
	// FolderTemplateID is the template id of the generic FOLDER template.
	FolderTemplateID = "generic_folder.v1"
)

var (
	//go:embed generic_ehr_status.v1.opt
	ehrStatusDocument []byte
	//go:embed generic_folder.v1.opt
	folderDocument []byte
)

var (
	ehrStatus = sync.OnceValue(func() *templatecompile.Compiled {
		return mustCompile(EHRStatusTemplateID, ehrStatusDocument)
	})
	folder = sync.OnceValue(func() *templatecompile.Compiled {
		return mustCompile(FolderTemplateID, folderDocument)
	})
)

// EHRStatusOPT returns the XML document of the generic EHR_STATUS operational
// template, byte for byte as the package embeds it. Each call returns a new
// slice, so a caller may change it without changing any other result.
func EHRStatusOPT() []byte {
	return bytes.Clone(ehrStatusDocument)
}

// FolderOPT returns the XML document of the generic FOLDER operational
// template, byte for byte as the package embeds it. Each call returns a new
// slice, so a caller may change it without changing any other result.
func FolderOPT() []byte {
	return bytes.Clone(folderDocument)
}

// EHRStatus returns the generic EHR_STATUS operational template, compiled.
// The first call compiles it; every later call, concurrent ones included,
// returns the same value, which callers share and must not change.
//
// EHRStatus panics only when the embedded document fails to read or
// compile, a defect of the SDK release that the package's own tests rule
// out; the panic value is an error that names the template id.
func EHRStatus() *templatecompile.Compiled {
	return ehrStatus()
}

// Folder returns the generic FOLDER operational template, compiled.
// The first call compiles it; every later call, concurrent ones included,
// returns the same value, which callers share and must not change.
//
// Folder panics only when the embedded document fails to read or compile,
// a defect of the SDK release that the package's own tests rule out; the
// panic value is an error that names the template id.
func Folder() *templatecompile.Compiled {
	return folder()
}

// mustCompile reads and compiles an embedded operational template document.
// It panics on failure, with an error that names id and wraps the cause:
// the document ships with the package, so a failure is a defect of the
// release, not of the caller's input.
func mustCompile(id string, document []byte) *templatecompile.Compiled {
	// REQ-178 § Surface: no error return, a panic naming the template id.
	operationalTemplate, err := aom14.ReadOperationalTemplate(bytes.NewReader(document))
	if err != nil {
		panic(fmt.Errorf("generic: read template %s: %w", id, err))
	}
	compiled, err := templatecompile.Compile(operationalTemplate)
	if err != nil {
		panic(fmt.Errorf("generic: compile template %s: %w", id, err))
	}
	return compiled
}
