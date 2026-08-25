package semcheck

import "github.com/cadasto/openehr-sdk-go/openehr/aql/contain"

// export_test.go re-exports the [Operand] internals the EXTERNAL test package
// (semcheck_test) needs to assert a decision, and that nothing else needs.
//
// They live here rather than in semcheck.go because the package doc's invariant
// — "neither adapter classifies a verdict of its own" — is what the shared
// engine exists to enforce: an exported Verdict() would hand an adapter the raw
// [contain.Verdict] to branch on, which is precisely the parity break REQ-162
// § Contract makes a MUST-not, and an exported RMType()/Suppresses() would offer
// an adapter the pieces to re-derive the suppression rule for itself. Neither
// adapter ever called any of the three; only the tests did. Keeping them
// test-only makes the invariant structural instead of merely discouraged, and
// costs the tests nothing — a file named *_test.go is compiled into this
// package for `go test` alone, so semcheck_test keeps full access while the
// shipped surface stays [Checker], [Operand], [Role], [Step], [Severity] and
// [SeverityOf].

// RMType is the class name this operand was decided from; "" for the zero
// Operand.
func (o Operand) RMType() string { return o.rmType }

// Verdict is the REQ-160 containability verdict of the operand's class:
// Admissible, Never, or UnknownClass. ByReference never appears here — it
// arises only on the pair question.
func (o Operand) Verdict() contain.Verdict { return o.verdict }

// Suppresses reports whether this operand suppresses the pair checks for every
// pair it participates in — see Operand.suppresses, which [Checker.Pair] uses.
func (o Operand) Suppresses() bool { return o.suppresses() }
