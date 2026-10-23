package instance

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_MultimediaURIOnlyWhenEmpty is the REQ-107 check that
// settleMultimedia gives a DV_MULTIMEDIA inline data, sized by its byte
// count, unless the OPT prohibits data, and the uri default
// http://example.com only where it then has neither uri nor data: a uri
// or inline data it already has is kept, and no uri is added beside data.
// It calls settleMultimedia directly, because a value Generate builds
// never has data before it is settled: no AOM 1.4 constraint describes
// data, an array of octets, so no OPT puts one there.
func TestREQ107_MultimediaURIOnlyWhenEmpty(t *testing.T) {
	node, _ := visitAttribute(t, visitRootOPT("DV_MULTIMEDIA", visitOptionalSingle("alternate_text")), "alternate_text")
	noData, _ := visitAttribute(t, visitRootOPT("DV_MULTIMEDIA", visitProhibitedSingle("data", "")), "data")
	t.Run("uri kept, data added", func(t *testing.T) {
		m := &rm.DVMultimedia{URI: &rm.DVURI{Value: "http://example.org/scan.png"}}
		settleMultimedia(node, m)
		if uri, ok := m.URI.(*rm.DVURI); !ok || uri.Value != "http://example.org/scan.png" {
			t.Errorf("settleMultimedia: uri = %#v, want the uri it had", m.URI)
		}
		if len(m.Data) == 0 || int(m.Size) != len(m.Data) {
			t.Errorf("settleMultimedia: data %q of size %d, want the example payload sized by its bytes", m.Data, m.Size)
		}
	})
	t.Run("data kept and sized, no uri added", func(t *testing.T) {
		data := []byte("text")
		m := &rm.DVMultimedia{Data: slices.Clone(data)}
		settleMultimedia(node, m)
		if m.URI != nil {
			t.Errorf("settleMultimedia: uri = %#v, want none beside data", m.URI)
		}
		if !slices.Equal(m.Data, data) || int(m.Size) != len(data) {
			t.Errorf("settleMultimedia: data %q of size %d, want %q of size %d", m.Data, m.Size, data, len(data))
		}
	})
	t.Run("neither: data added, no uri", func(t *testing.T) {
		m := &rm.DVMultimedia{}
		settleMultimedia(node, m)
		if m.URI != nil {
			t.Errorf("settleMultimedia: uri = %#v, want none beside data", m.URI)
		}
		if len(m.Data) == 0 || int(m.Size) != len(m.Data) {
			t.Errorf("settleMultimedia: data %q of size %d, want the example payload sized by its bytes", m.Data, m.Size)
		}
	})
	t.Run("neither, data prohibited: uri added", func(t *testing.T) {
		m := &rm.DVMultimedia{}
		settleMultimedia(noData, m)
		if len(m.Data) != 0 || m.Size != 0 {
			t.Errorf("settleMultimedia: data %q of size %d, want none of size 0", m.Data, m.Size)
		}
		if uri, ok := m.URI.(*rm.DVURI); !ok || uri.Value != "http://example.com" {
			t.Errorf("settleMultimedia: uri = %#v, want http://example.com", m.URI)
		}
	})
	t.Run("two values do not share the payload", func(t *testing.T) {
		first, second := &rm.DVMultimedia{}, &rm.DVMultimedia{}
		settleMultimedia(node, first)
		settleMultimedia(node, second)
		if len(first.Data) == 0 {
			t.Fatal("settleMultimedia wrote no data")
		}
		first.Data[0] ^= 0xff
		if first.Data[0] == second.Data[0] {
			t.Errorf("settleMultimedia: changing one value's data changed another's")
		}
	})
}
