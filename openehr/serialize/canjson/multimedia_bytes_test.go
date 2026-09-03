package canjson_test

import (
	"bytes"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052: DV_MULTIMEDIA.data / integrity_check ([]byte) round-trip as base64.
func TestDVMultimediaBytesRoundTrip(t *testing.T) {
	// "ghggjgjggj" -> base64 "Z2hnZ2pnamdnag==" (matches the FLAT conformance fixture).
	in := []byte(`{"_type":"DV_MULTIMEDIA",` +
		`"media_type":{"_type":"CODE_PHRASE","terminology_id":` +
		`{"_type":"TERMINOLOGY_ID","value":"IANA_media-types"},"code_string":"text/plain"},` +
		`"size":10,"data":"Z2hnZ2pnamdnag==","integrity_check":"Z2hnZ2pnamdnag=="}`)
	var m rm.DVMultimedia
	if err := canjson.Unmarshal(in, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(m.Data) != "ghggjgjggj" {
		t.Fatalf("data = %q, want decoded bytes", string(m.Data))
	}
	if string(m.IntegrityCheck) != "ghggjgjggj" {
		t.Fatalf("integrity_check = %q, want decoded bytes", string(m.IntegrityCheck))
	}
	out, err := canjson.Marshal(&m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(out, []byte(`"data":"Z2hnZ2pnamdnag=="`)) {
		t.Fatalf("encoded form = %s, want base64 data", out)
	}
	if !bytes.Contains(out, []byte(`"integrity_check":"Z2hnZ2pnamdnag=="`)) {
		t.Fatalf("encoded form = %s, want base64 integrity_check", out)
	}
}
