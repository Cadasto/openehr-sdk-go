package terminology

// Temporary: the registry tables the accessors range over, empty until the
// generated table exists. The termgen generator declares these same two
// variables in openehr_gen.go, populated from the pin, and this file goes
// away when it lands.

var (
	groups   []*Group
	codeSets []*CodeSet
)
