package smart

import "testing"

// TestPrincipalFromClaims_DefensiveCopy verifies that mutating the original
// claims map after calling principalFromClaims does not affect the stored
// PrincipalIdentity.Raw (i.e. Raw is a defensive copy, not an alias).
func TestPrincipalFromClaims_DefensiveCopy(t *testing.T) {
	claims := map[string]any{
		"principal_uid":  "u1",
		"principal_type": "PERSON",
		"x":              "y",
	}
	p := principalFromClaims(claims, PrincipalClaimNames{})
	if p == nil {
		t.Fatal("expected non-nil PrincipalIdentity")
	}
	if p.UID != "u1" {
		t.Fatalf("UID = %q, want %q", p.UID, "u1")
	}

	// Mutate the original claims map.
	claims["principal_uid"] = "HACKED"
	delete(claims, "x")
	claims["injected"] = "bad"

	// The stored Raw must be unaffected.
	if got, ok := p.Raw["principal_uid"]; !ok || got != "u1" {
		t.Errorf("Raw[principal_uid] = %v, want %q (mutation leaked into Raw)", got, "u1")
	}
	if _, ok := p.Raw["injected"]; ok {
		t.Error("injected key appeared in Raw — Raw is not a defensive copy")
	}
	if got, ok := p.Raw["x"]; !ok || got != "y" {
		t.Errorf("Raw[x] = %v, want %q (deleted key missing from Raw)", got, "y")
	}
}
