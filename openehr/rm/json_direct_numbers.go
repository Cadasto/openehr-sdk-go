package rm

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"math"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// The generated canonical-JSON encoders of this package write a member held
// as a Real, an Integer or an Integer64 through the writers below. Each
// writes the bytes the type's MarshalJSON returns straight to the encoder,
// without the call and the slice it returns. They are functions rather than
// MarshalJSONTo methods: json/v2 and encoding/json would prefer such a method,
// promoted, to the MarshalJSON of a struct that embeds the type.

// writeJSONReal writes r as the JSON number [Real.MarshalJSON] returns. A NaN
// or an infinity, which JSON cannot spell, goes to json.MarshalEncode, which
// refuses it as it refuses r inside any other value.
func writeJSONReal(enc *jsontext.Encoder, r Real) error {
	f := float64(r)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return json.MarshalEncode(enc, r, typereg.MarshalOptions(enc))
	}
	var buf [32]byte
	return enc.WriteValue(appendJSONReal(buf[:0], f))
}

// writeJSONInteger writes i as the JSON number [Integer.MarshalJSON] returns.
func writeJSONInteger(enc *jsontext.Encoder, i Integer) error {
	var buf [16]byte
	return enc.WriteValue(strconv.AppendInt(buf[:0], int64(i), 10))
}

// writeJSONInteger64 writes i as the JSON number [Integer64.MarshalJSON]
// returns.
func writeJSONInteger64(enc *jsontext.Encoder, i Integer64) error {
	var buf [24]byte
	return enc.WriteValue(strconv.AppendInt(buf[:0], int64(i), 10))
}

// appendJSONReal appends f to dst in the form encoding/json/v2 marshals a
// finite float64 in: the shortest decimal that reads back as f, written
// without an exponent from 1e-6 up to but excluding 1e21 and with one outside
// that range, its exponent stripped of a leading zero (1e-7, not 1e-07).
func appendJSONReal(dst []byte, f float64) []byte {
	format := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	dst = strconv.AppendFloat(dst, f, format, -1, 64)
	if n := len(dst); format == 'e' && n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
		dst[n-2] = dst[n-1]
		dst = dst[:n-1]
	}
	return dst
}
