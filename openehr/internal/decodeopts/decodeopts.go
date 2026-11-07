// Package decodeopts tells the operational template reader of
// openehr/aom/aom14 what a list of canonical-XML decode options asks for,
// where the reader has to know it before it decodes, and gives the reader the
// one option it passes its first decode: an observer of the tokens that decode
// reads. canxml keeps what its options set unexported; it registers here, when
// it initialises, the function that reads them and the one that makes that
// option, so the reader needs no public API for either.
package decodeopts

import "encoding/xml"

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

// observer makes the decode option [Observer] returns; nil until canxml
// registers it.
var observer func(fn func(xml.Token)) any

// RegisterObserver makes fn the function that makes the decode option
// [Observer] returns. canxml calls it once, from an init function, before any
// decode can run.
func RegisterObserver(fn func(func(xml.Token)) any) {
	observer = fn
}

// Observer returns a canonical-XML decode option that passes fn each token the
// decode reads from its source, in order and once, before the decode acts on
// it: element names with their namespaces resolved, and every attribute, the
// namespace declarations included. fn must not keep a token's bytes past the
// call. It returns nil while no function is registered.
func Observer(fn func(xml.Token)) any {
	if observer == nil {
		return nil
	}
	return observer(fn)
}
