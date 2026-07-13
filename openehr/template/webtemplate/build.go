package webtemplate

import "github.com/cadasto/openehr-sdk-go/openehr/templatecompile"

// Build projects a compiled OPT into the typed WebTemplate tree (REQ-106).
//
// TODO(Task 3): replace this stub with the recursive transform.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error) {
	if c == nil || c.Root() == nil {
		return nil, ErrEmptyTemplate
	}
	return nil, ErrEmptyTemplate
}
