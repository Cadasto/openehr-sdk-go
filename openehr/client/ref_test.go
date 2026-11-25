package client_test

import (
	"errors"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestRefConstruction(t *testing.T) {
	if r := openehrclient.LatestOf("voID"); r.PathSegment() != "voID" {
		t.Errorf("LatestOf PathSegment = %q", r.PathSegment())
	}
	if r := openehrclient.VersionOf("uid::s::1"); r.PathSegment() != "uid::s::1" {
		t.Errorf("VersionOf PathSegment = %q", r.PathSegment())
	}
	at, err := time.Parse(time.RFC3339, "2026-05-17T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	r := openehrclient.LatestAtTime("voID", at)
	k, v := r.Query()
	if k != "version_at_time" {
		t.Errorf("LatestAtTime Query key = %q, want version_at_time", k)
	}
	if v != "2026-05-17T10:00:00Z" {
		t.Errorf("LatestAtTime Query value = %q", v)
	}
}

// REQ-174: a Ref that carries a sub-second time sends version_at_time
// in UTC with its fraction of a second.
func TestLatestAtTimeQueryKeepsFraction(t *testing.T) {
	at := time.Date(2015, 1, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600))
	k, v := openehrclient.LatestAtTime("voID", at).Query()
	if k != "version_at_time" {
		t.Errorf("LatestAtTime Query key = %q, want version_at_time", k)
	}
	if want := "2015-01-20T18:30:22.765Z"; v != want {
		t.Errorf("LatestAtTime Query value = %q, want %q", v, want)
	}
}

// REQ-174: a Ref whose time has a UTC year outside 0000-9999 fails
// Validate with ErrInvalidConfig, and its Query carries no value; every
// other Ref validates.
func TestRefValidate(t *testing.T) {
	// 31 December 9999 at -05:00 is the year 10000 in UTC.
	year10000 := time.Date(9999, 12, 31, 23, 59, 59, 0, time.FixedZone("", -5*3600))
	normal := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		ref  openehrclient.Ref
		want error
	}{
		{name: "LatestAtTime/UTC year 10000", ref: openehrclient.LatestAtTime("voID", year10000), want: transport.ErrInvalidConfig},
		{name: "LatestOf", ref: openehrclient.LatestOf("voID")},
		{name: "LatestAtTime/2026", ref: openehrclient.LatestAtTime("voID", normal)},
		{name: "VersionOf", ref: openehrclient.VersionOf("uid::s::1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.ref.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("%s Validate() = %v, want %v", tc.name, err, tc.want)
			}
		})
	}
	if k, v := openehrclient.LatestAtTime("voID", year10000).Query(); k != "version_at_time" || v != "" {
		t.Errorf("LatestAtTime(year 10000) Query() = %q, %q, want \"version_at_time\", \"\"", k, v)
	}
}
