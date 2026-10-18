package jwtbearer

import "testing"

// TestREQ026_ClaimsSignerKeepsItsOwnExtraClaims pins REQ-026 (idiom.md §
// Concurrency) with REQ-068: NewClaimsSigner keeps its own copy of the
// template's Extra map, so a claim the caller adds, replaces or removes in
// its map afterwards does not reach the assertions the signer signs, and
// does not race with Assertion reading the map.
func TestREQ026_ClaimsSignerKeepsItsOwnExtraClaims(t *testing.T) {
	extra := map[string]any{"scope": "system/*.rs", "tenant": "t1"}
	signer, err := NewClaimsSigner(ClaimsTemplate{
		Issuer:   "client-xyz",
		Audience: "https://auth.example.com/token",
		Extra:    extra,
	}, newKey(t))
	if err != nil {
		t.Fatalf("NewClaimsSigner() error = %v", err)
	}
	extra["scope"] = "changed"
	delete(extra, "tenant")
	extra["added"] = true

	jwt, err := signer.Assertion(t.Context())
	if err != nil {
		t.Fatalf("Assertion() error = %v", err)
	}
	_, claims, _ := decodeJWT(t, jwt)
	if claims["scope"] != "system/*.rs" || claims["tenant"] != "t1" {
		t.Errorf("assertion claims scope = %v, tenant = %v; want system/*.rs, t1 as the template had them", claims["scope"], claims["tenant"])
	}
	if _, ok := claims["added"]; ok {
		t.Errorf("assertion claims carry %q, which the caller added to its map after NewClaimsSigner", "added")
	}
}
