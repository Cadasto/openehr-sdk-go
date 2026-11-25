package ehr

// This file is in package ehr, not ehr_test, because it uses the deprecated
// names in shared.go on purpose: staticcheck's SA1019 does not flag a use
// inside the declaring package.

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Each deprecated type in shared.go is an alias of its package client type.
// A pointer to one named type never assigns to a pointer to another, and an
// interface's pointer does not either, so every line fails to compile when
// its alias becomes a defined type. The compiler is the test: there is no
// run-time part.
var (
	_ *client.VersionedObjectID     = (*VersionedObjectID)(nil)
	_ *client.VersionUID            = (*VersionUID)(nil)
	_ *client.VersionMetadata       = (*VersionMetadata)(nil)
	_ *client.Identifier            = (*Identifier)(nil)
	_ *client.Ref                   = (*Ref)(nil)
	_ *client.ByVersionedObjectID   = (*ByVersionedObjectID)(nil)
	_ *client.ByVersionUID          = (*ByVersionUID)(nil)
	_ *client.ItemTag               = (*ItemTag)(nil)
	_ *client.StoredItemTag         = (*StoredItemTag)(nil)
	_ *client.LifecycleState        = (*LifecycleState)(nil)
	_ *client.WriteConfig           = (*WriteConfig)(nil)
	_ *client.NoRepresentationError = (*NoRepresentationError)(nil)
)

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestSharedLifecycleStateConstants pins each deprecated lifecycle state
// code to its package client value (module-layout.md, openehr/client/ehr/).
func TestSharedLifecycleStateConstants(t *testing.T) {
	cases := []struct {
		name string
		got  client.LifecycleState
		want client.LifecycleState
	}{
		{name: "complete", got: LifecycleStateComplete, want: client.LifecycleStateComplete},
		{name: "incomplete", got: LifecycleStateIncomplete, want: client.LifecycleStateIncomplete},
		{name: "deleted", got: LifecycleStateDeleted, want: client.LifecycleStateDeleted},
		{name: "inactive", got: LifecycleStateInactive, want: client.LifecycleStateInactive},
		{name: "abandoned", got: LifecycleStateAbandoned, want: client.LifecycleStateAbandoned},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("deprecated LifecycleState%s = %q, want %q (the client constant)", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestSharedRefWrappers pins LatestOf, LatestAtTime and VersionOf to what the
// package client function answers for the same input. A Ref holds a
// comparable struct, so == compares the dynamic type and the value.
func TestSharedRefWrappers(t *testing.T) {
	at := time.Date(2015, time.January, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600))

	t.Run("LatestOf", func(t *testing.T) {
		for _, id := range []client.VersionedObjectID{"obj-1", ""} {
			if got, want := LatestOf(id), client.LatestOf(id); got != want {
				t.Errorf("LatestOf(%q) = %#v, want %#v (what client.LatestOf returned)", id, got, want)
			}
		}
	})

	t.Run("LatestAtTime", func(t *testing.T) {
		cases := []struct {
			name string
			id   client.VersionedObjectID
			at   time.Time
		}{
			{name: "an instant", id: "obj-1", at: at},
			{name: "the zero time", id: "obj-1"},
			{name: "year 10000", id: "obj-1", at: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got, want := LatestAtTime(tc.id, tc.at), client.LatestAtTime(tc.id, tc.at); got != want {
					t.Errorf("LatestAtTime(%q, %v) = %#v, want %#v (what client.LatestAtTime returned)", tc.id, tc.at, got, want)
				}
			})
		}
	})

	t.Run("VersionOf", func(t *testing.T) {
		for _, uid := range []client.VersionUID{"obj-1::sys::2", ""} {
			if got, want := VersionOf(uid), client.VersionOf(uid); got != want {
				t.Errorf("VersionOf(%q) = %#v, want %#v (what client.VersionOf returned)", uid, got, want)
			}
		}
	})
}

// TestSharedNewVersionMetadata pins NewVersionMetadata to the package client
// function, on nil and on a response that names a version.
func TestSharedNewVersionMetadata(t *testing.T) {
	cases := []struct {
		name string
		in   *transport.Metadata
	}{
		{name: "nil", in: nil},
		{name: "etag and location", in: &transport.Metadata{
			ETag:     "obj-1::sys::2",
			ETags:    []string{"obj-1::sys::2"},
			Location: "/ehr/ehr-1/composition/obj-1::sys::2",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, want := NewVersionMetadata(tc.in), client.NewVersionMetadata(tc.in)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("NewVersionMetadata(%+v) = %+v, want %+v (what client.NewVersionMetadata returned)", tc.in, got, want)
			}
		})
	}
}

// TestSharedHasResource pins HasResource to the package client function, on a
// typed nil and on a present RM pointer.
func TestSharedHasResource(t *testing.T) {
	cases := []struct {
		name string
		in   *rm.Composition
	}{
		{name: "nil", in: nil},
		{name: "present", in: &rm.Composition{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := HasResource(tc.in), client.HasResource(tc.in); got != want {
				t.Errorf("HasResource(%#v) = %v, want %v (what client.HasResource returned)", tc.in, got, want)
			}
		})
	}
}

// TestSharedMarshalAuditDetails pins MarshalAuditDetails to the package
// client function, on nil, on a small AUDIT_DETAILS and on one the header
// guard refuses.
func TestSharedMarshalAuditDetails(t *testing.T) {
	name := "Alice"
	small := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "251"}},
		Committer:  rm.PartyIdentified{Name: &name},
		SystemID:   "cdr.example",
	}
	controlChar := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "251"}},
		Committer:  rm.PartyIdentified{Name: &name},
		SystemID:   "cdr\nexample",
	}
	cases := []struct {
		name string
		in   *rm.AuditDetails
	}{
		{name: "nil", in: nil},
		{name: "small", in: small},
		{name: "control character", in: controlChar},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := MarshalAuditDetails(tc.in)
			want, wantErr := client.MarshalAuditDetails(tc.in)
			if got != want {
				t.Errorf("MarshalAuditDetails(%s) = %q, want %q (what client.MarshalAuditDetails returned)", tc.name, got, want)
			}
			if errText(gotErr) != errText(wantErr) {
				t.Errorf("MarshalAuditDetails(%s) err = %v, want %v (what client.MarshalAuditDetails returned)", tc.name, gotErr, wantErr)
			}
		})
	}
}

// TestSharedItemTagHeaders pins FormatItemTagHeader and ParseItemTagHeader to
// the package client functions, on one valid and one invalid input each.
func TestSharedItemTagHeaders(t *testing.T) {
	t.Run("FormatItemTagHeader", func(t *testing.T) {
		cases := []struct {
			name string
			in   []client.ItemTag
		}{
			{name: "valid", in: []client.ItemTag{{Key: "category", Value: "final", TargetPath: "/path"}}},
			{name: "empty key", in: []client.ItemTag{{Key: ""}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, gotErr := FormatItemTagHeader(tc.in)
				want, wantErr := client.FormatItemTagHeader(tc.in)
				if got != want {
					t.Errorf("FormatItemTagHeader(%+v) = %q, want %q (what client.FormatItemTagHeader returned)", tc.in, got, want)
				}
				if errText(gotErr) != errText(wantErr) {
					t.Errorf("FormatItemTagHeader(%+v) err = %v, want %v (what client.FormatItemTagHeader returned)", tc.in, gotErr, wantErr)
				}
			})
		}
	})

	t.Run("ParseItemTagHeader", func(t *testing.T) {
		cases := []struct {
			name   string
			header string
		}{
			{name: "valid", header: `key="category",value="final",target_path="/path"`},
			{name: "unknown field", header: `bogus="x"`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, gotErr := ParseItemTagHeader(tc.header)
				want, wantErr := client.ParseItemTagHeader(tc.header)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ParseItemTagHeader(%q) = %+v, want %+v (what client.ParseItemTagHeader returned)", tc.header, got, want)
				}
				if errText(gotErr) != errText(wantErr) {
					t.Errorf("ParseItemTagHeader(%q) err = %v, want %v (what client.ParseItemTagHeader returned)", tc.header, gotErr, wantErr)
				}
			})
		}
	})
}

// TestSharedFormatVersionAtTime pins FormatVersionAtTime to the package
// client function, on a normal time and on the year 10000, which RFC 3339
// cannot write.
func TestSharedFormatVersionAtTime(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
	}{
		{name: "normal", in: time.Date(2015, time.January, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600))},
		{name: "year 10000 UTC", in: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := FormatVersionAtTime(tc.in)
			want, wantErr := client.FormatVersionAtTime(tc.in)
			if got != want {
				t.Errorf("FormatVersionAtTime(%v) = %q, want %q (what client.FormatVersionAtTime returned)", tc.in, got, want)
			}
			if errText(gotErr) != errText(wantErr) {
				t.Errorf("FormatVersionAtTime(%v) err = %v, want %v (what client.FormatVersionAtTime returned)", tc.in, gotErr, wantErr)
			}
		})
	}
}

// TestSharedFormatLifecycleStateHeader pins FormatLifecycleStateHeader to the
// package client function, on a valid state and on an unknown one.
func TestSharedFormatLifecycleStateHeader(t *testing.T) {
	cases := []struct {
		name string
		in   client.LifecycleState
	}{
		{name: "valid", in: client.LifecycleStateComplete},
		{name: "unknown", in: "999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := FormatLifecycleStateHeader(tc.in)
			want, wantErr := client.FormatLifecycleStateHeader(tc.in)
			if got != want {
				t.Errorf("FormatLifecycleStateHeader(%q) = %q, want %q (what client.FormatLifecycleStateHeader returned)", tc.in, got, want)
			}
			if errText(gotErr) != errText(wantErr) {
				t.Errorf("FormatLifecycleStateHeader(%q) err = %v, want %v (what client.FormatLifecycleStateHeader returned)", tc.in, gotErr, wantErr)
			}
		})
	}
}

// TestSharedWriteResult pins WriteResult to the package client function.
// Both calls use a nil client, so each fails before any request: the first
// row on the nil decode of a representation write, the second on the nil
// client itself. Each must wrap ErrInvalidConfig, so a pass is no vacuous
// agreement of two successes.
func TestSharedWriteResult(t *testing.T) {
	cases := []struct {
		name   string
		prefer transport.Prefer
	}{
		{name: "representation with a nil decode", prefer: transport.PreferRepresentation},
		{name: "minimal with a nil client", prefer: transport.PreferMinimal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &transport.Request{
				Method: http.MethodPost,
				Path:   "/ehr/ehr-1/composition",
				Route:  "/ehr/{ehr_id}/composition",
				Prefer: tc.prefer,
			}
			got, gotMeta, gotErr := WriteResult[*rm.Composition](t.Context(), nil, req, "composition", nil)
			want, wantMeta, wantErr := client.WriteResult[*rm.Composition](t.Context(), nil, req, "composition", nil)

			if !errors.Is(wantErr, transport.ErrInvalidConfig) {
				t.Fatalf("client.WriteResult err = %v, want one wrapping ErrInvalidConfig", wantErr)
			}
			if !errors.Is(gotErr, transport.ErrInvalidConfig) {
				t.Errorf("WriteResult err = %v, want one wrapping ErrInvalidConfig", gotErr)
			}
			if errText(gotErr) != errText(wantErr) {
				t.Errorf("WriteResult err = %v, want %v (what client.WriteResult returned)", gotErr, wantErr)
			}
			if got != want {
				t.Errorf("WriteResult resource = %#v, want %#v (what client.WriteResult returned)", got, want)
			}
			if !reflect.DeepEqual(gotMeta, wantMeta) {
				t.Errorf("WriteResult metadata = %+v, want %+v (what client.WriteResult returned)", gotMeta, wantMeta)
			}
		})
	}
}

// TestSharedDoDelete pins DoDelete to the package client function: a nil
// client answers the same ErrInvalidConfig from both.
func TestSharedDoDelete(t *testing.T) {
	req := &transport.Request{
		Method: http.MethodDelete,
		Path:   "/ehr/ehr-1/composition/obj-1::sys::1",
		Route:  "/ehr/{ehr_id}/composition/{uid}",
	}
	gotMeta, gotErr := DoDelete(t.Context(), nil, req)
	wantMeta, wantErr := client.DoDelete(t.Context(), nil, req)

	if !errors.Is(wantErr, transport.ErrInvalidConfig) {
		t.Fatalf("client.DoDelete err = %v, want one wrapping ErrInvalidConfig", wantErr)
	}
	if !errors.Is(gotErr, transport.ErrInvalidConfig) {
		t.Errorf("DoDelete err = %v, want one wrapping ErrInvalidConfig", gotErr)
	}
	if errText(gotErr) != errText(wantErr) {
		t.Errorf("DoDelete err = %v, want %v (what client.DoDelete returned)", gotErr, wantErr)
	}
	if !reflect.DeepEqual(gotMeta, wantMeta) {
		t.Errorf("DoDelete metadata = %+v, want %+v (what client.DoDelete returned)", gotMeta, wantMeta)
	}
}
