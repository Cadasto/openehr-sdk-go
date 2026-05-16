package bmmgen

import (
	"context"
	"io"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// wrappedResolver delegates to a [bmm.FSResolver] but typed as the
// generator-local concrete value so cmd/bmmgen can construct it
// without re-exposing the BMM concrete resolver type.
type wrappedResolver struct {
	root string
}

func (w wrappedResolver) Resolve(ctx context.Context, id string) (io.ReadCloser, error) {
	return bmm.FSResolver{Root: w.root}.Resolve(ctx, id)
}
