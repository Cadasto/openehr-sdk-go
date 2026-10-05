package instance

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_MultimediaURIOnlyWhenEmpty is the REQ-107 check that the uri
// default http://example.com goes only on a DV_MULTIMEDIA with neither uri
// nor data: a uri or inline data it already has is kept, and no uri is
// added beside data. It calls settleMultimedia directly, because the
// template-instance writer cannot write a DV_MULTIMEDIA's uri or data, so
// a value Generate builds never has either.
func TestREQ107_MultimediaURIOnlyWhenEmpty(t *testing.T) {
	node, _ := visitAttribute(t, visitRootOPT("DV_MULTIMEDIA", visitOptionalSingle("alternate_text")), "alternate_text")
	t.Run("uri kept", func(t *testing.T) {
		m := &rm.DVMultimedia{URI: &rm.DVURI{Value: "http://example.org/scan.png"}}
		settleMultimedia(node, m)
		if uri, ok := m.URI.(*rm.DVURI); !ok || uri.Value != "http://example.org/scan.png" {
			t.Errorf("settleMultimedia: uri = %#v, want the uri it had", m.URI)
		}
	})
	t.Run("data kept, no uri added", func(t *testing.T) {
		data := []byte("text")
		m := &rm.DVMultimedia{Data: slices.Clone(data)}
		settleMultimedia(node, m)
		if m.URI != nil {
			t.Errorf("settleMultimedia: uri = %#v, want none beside data", m.URI)
		}
		if !slices.Equal(m.Data, data) {
			t.Errorf("settleMultimedia: data = %q, want %q", m.Data, data)
		}
	})
	t.Run("neither: uri added", func(t *testing.T) {
		m := &rm.DVMultimedia{}
		settleMultimedia(node, m)
		if uri, ok := m.URI.(*rm.DVURI); !ok || uri.Value != "http://example.com" {
			t.Errorf("settleMultimedia: uri = %#v, want http://example.com", m.URI)
		}
	})
}
