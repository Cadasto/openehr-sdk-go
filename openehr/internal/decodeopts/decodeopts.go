// Package decodeopts tells the operational template reader of
// openehr/aom/aom14 what a list of canonical-XML decode options asks for,
// where the reader has to know it before it decodes. canxml keeps what its
// options set unexported; it registers here, when it initialises, the one
// function that reads it, so the reader learns it without a public API for it.
package decodeopts

// Settings is what a list of canonical-XML decode options asks for that the
// operational template reader has to know.
type Settings struct {
	// Strict is true when the options refuse unmatched content.
	Strict bool
	// Reports is true when the options pass unmatched content, the mandatory
	// members the input leaves out, or the members it sends without a value,
	// to a function of the caller's.
	Reports bool
	// MaxDepth is the depth limit the options set, the smallest positive one
	// they give, or 0 when they set none.
	MaxDepth int
}

// read returns the Settings of a list of options; nil until canxml registers
// it.
var read func(opts any) Settings

// Register makes fn the function that reads the Settings of a list of
// options. canxml calls it once, from an init function, before any decode can
// run.
func Register(fn func(opts any) Settings) {
	read = fn
}

// Of returns the Settings of opts, a list of canonical-XML decode options, or
// the zero Settings while no function is registered.
func Of(opts any) Settings {
	if read == nil {
		return Settings{}
	}
	return read(opts)
}
