package demographic

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Format names the representation a PARTY, or a versioned_party resource,
// travels in on the wire. The zero value is [FormatJSON], canonical JSON,
// which every call uses unless an option names another format.
//
// The simplified FLAT and STRUCTURED formats are not offered for a party:
// the SDK's simplified codec reads and writes compositions only.
type Format int

const (
	// FormatJSON is canonical JSON, sent as application/json. It is the
	// zero value and the default.
	FormatJSON Format = iota
	// FormatXML is canonical XML, sent as application/xml.
	FormatXML
)

const (
	mediaTypeJSON = "application/json"
	mediaTypeXML  = "application/xml"
)

// MediaType returns the media type f is sent as. A value other than
// [FormatJSON] and [FormatXML] returns "".
func (f Format) MediaType() string {
	switch f {
	case FormatJSON:
		return mediaTypeJSON
	case FormatXML:
		return mediaTypeXML
	default:
		return ""
	}
}

// String names the format: "JSON" or "XML". Any other value is named by its
// number.
func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "JSON"
	case FormatXML:
		return "XML"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// EncodeError reports a party that the codec of Format cannot serialise.
// [Create] and [Update] return it before sending any request, so a caller
// can tell "this party cannot go in this format" apart from a request that
// went out and failed, and from an option refused with
// [transport.ErrInvalidConfig].
//
// Err is the codec's own error, which [errors.Is] and [errors.AsType] reach
// through Unwrap, such as [canjson.ErrInvalidValue].
type EncodeError struct {
	// Format is the format the write asked for.
	Format Format
	// Err is the error the codec returned.
	Err error
}

// Error names the format and, after a colon, the codec's error. A nil
// receiver gives the text of the zero value.
func (e *EncodeError) Error() string {
	if e == nil {
		e = &EncodeError{}
	}
	msg := "encode the party as " + e.Format.String()
	if e.Err == nil {
		return msg
	}
	return msg + ": " + e.Err.Error()
}

// Unwrap returns the codec's error, or nil for a nil receiver.
func (e *EncodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// valid reports whether f is one of the two formats.
func (f Format) valid() bool {
	switch f {
	case FormatJSON, FormatXML:
		return true
	default:
		return false
	}
}

// marshal serialises party in f. Callers refuse a format outside the two
// with valid first, so the default arm is unreachable; it fails loudly
// rather than send an empty body.
func (f Format) marshal(party rm.Party) ([]byte, error) {
	switch f {
	case FormatJSON:
		return canjson.Marshal(party)
	case FormatXML:
		return canxml.Marshal(addressable(party))
	default:
		return nil, fmt.Errorf("demographic: no codec for %s", f)
	}
}

// decodeParty decodes body, a bare PARTY, as the class t names, in f.
// Canonical JSON decodes the class its `_type` names and refuses one other
// than t's; canonical XML decodes t's class and refuses a root element whose
// xsi:type names another. The codec's error is returned unwrapped. Callers
// refuse a format outside the two with valid first, so the default arm is
// unreachable.
func (f Format) decodeParty(body []byte, t Type) (rm.Party, error) {
	switch f {
	case FormatJSON:
		// REQ-173: the class the call names decides in canonical JSON as in
		// XML; a `_type` naming another PARTY class is a decode failure
		// (REQ-151), never a silent decode of the wrong class.
		party, err := typereg.DecodeAs[rm.Party](body)
		if err != nil {
			return nil, err
		}
		if got, _ := resourceType(party); got != t {
			return nil, fmt.Errorf("demographic: the body's _type names %T, not the %s the call names: %w", party, strings.ToUpper(string(t)), typereg.ErrTypeMismatch)
		}
		return party, nil
	case FormatXML:
		party := newParty(t)
		if party == nil {
			return nil, fmt.Errorf("demographic: no PARTY class for type %q", t)
		}
		if err := unmarshalXML(body, party); err != nil {
			return nil, err
		}
		return party, nil
	default:
		return nil, errors.New("no codec for this format")
	}
}

// unmarshal decodes body into out, a concrete RM value, in f, and returns
// the codec's own error unwrapped. Callers refuse a format outside the two
// with valid first, so the default arm is unreachable.
func (f Format) unmarshal(body []byte, out any) error {
	switch f {
	case FormatJSON:
		return canjson.Unmarshal(body, out)
	case FormatXML:
		return unmarshalXML(body, out)
	default:
		return errors.New("no codec for this format")
	}
}

// addressable returns a PARTY given by value as a pointer to a copy. The
// canonical XML encoder's methods have pointer receivers, while the write
// path accepts a party by value as canonical JSON does.
func addressable(p rm.Party) rm.Party {
	switch v := p.(type) {
	case rm.Person:
		return &v
	case rm.Organisation:
		return &v
	case rm.Group:
		return &v
	case rm.Agent:
		return &v
	case rm.Role:
		return &v
	default:
		return p
	}
}

// newParty returns a new, empty PARTY of the class t names, or nil for a
// type outside the five.
func newParty(t Type) rm.Party {
	switch t {
	case Person:
		return new(rm.Person)
	case Organisation:
		return new(rm.Organisation)
	case Group:
		return new(rm.Group)
	case Agent:
		return new(rm.Agent)
	case Role:
		return new(rm.Role)
	default:
		return nil
	}
}

// unmarshalXML decodes body, one canonical XML document, into out, the
// class the call names. The codec matches the root element neither by name
// nor by type, so the root is checked first, before the body is decoded: a
// root naming a class other than out's would otherwise decode into out
// without complaint, or fail on a child with an error that hides the cause.
// The root names its class by its discriminator, read as the codec reads one
// at a polymorphic site, or, with none, by its element name, which canxml
// writes as the class's lower-cased name. A discriminator that names no
// openEHR class, a namespace-prefixed one included, is refused wrapping
// typereg.ErrUnknownType, and another class wrapping
// typereg.ErrTypeMismatch. A root whose name names no class, such as
// `party` or `version`, is read as out's class.
func unmarshalXML(body []byte, out any) error {
	// REQ-173: the class the call names decides, and a root naming another
	// is a decode failure (REQ-151), never a silent decode; REQ-056 decides
	// how the xsi:type value reads. REQ-174 keys the IMPORTED_VERSION
	// refusal, which an `imported_version` root names.
	want, ok := canxml.BMMNameOf(out)
	if !ok {
		return fmt.Errorf("demographic: %T has no openEHR class name", out)
	}
	if err := checkRoot(body, want); err != nil {
		return err
	}
	return canxml.Unmarshal(body, out)
}

// checkRoot refuses the root element of body when it names a class other
// than want, as unmarshalXML describes.
func checkRoot(body []byte, want string) error {
	start, err := rootElement(body)
	if err != nil {
		return err
	}
	got, err := canxml.XSITypeOf(start)
	if err != nil {
		return err
	}
	if got == "" {
		got = classOfRootName(start.Name.Local)
	}
	if got == "" || got == want {
		return nil
	}
	if _, known := typereg.Default.Lookup(got); !known {
		return fmt.Errorf("demographic: the root element's xsi:type names no openEHR class: %w", typereg.ErrUnknownType)
	}
	return fmt.Errorf("demographic: the root element names %s, not the %s the call names: %w", got, want, typereg.ErrTypeMismatch)
}

// classOfRootName returns the registered class whose canonical XML root
// element canxml names local, or "" when local names none.
func classOfRootName(local string) string {
	class := strings.ToUpper(local)
	if _, known := typereg.Default.Lookup(class); !known || canxml.ElementName(class) != local {
		return ""
	}
	return class
}

// rootElement returns the start tag of the root element of body. The root
// has no enclosing element, so the namespace declarations it carries itself
// are all that is in scope for its discriminator.
func rootElement(body []byte) (xml.StartElement, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start, nil
		}
	}
}

// WithFormat sets the representation the party is written in, and the one a
// returned party is read in. The default is [FormatJSON]. When the option is
// given more than once, the last one wins.
//
// The request body is serialised in f and sent with f's media type as
// `Content-Type`. Under [transport.PreferRepresentation] the write asks for
// f's media type in `Accept` and decodes the response body in f; under any
// other preference it asks for application/json, the media type of the
// identifier body the server may return. A canonical XML body is decoded as
// the class written, and one whose root element carries an `xsi:type`
// naming another class does not decode.
//
// The write fails with [transport.ErrInvalidConfig], before any request,
// when f is neither [FormatJSON] nor [FormatXML]. A party the codec cannot
// serialise also fails before any request, with an [*EncodeError] that
// names f and wraps the codec's error.
func WithFormat(f Format) WriteOption {
	return func(c *writeConfig) { c.format = f }
}

// formatWrite is the part of a Create or Update request that the
// representation format decides.
type formatWrite struct {
	body        []byte
	contentType string
	accept      string
	decode      func([]byte) (rm.Party, error)
}

// resolveFormat applies the format option to one write of party, a PARTY
// of type t, and serialises it. Both refusals come before any request: a
// format outside the two, and a party the codec cannot serialise, returned
// as an *EncodeError.
func (c *writeConfig) resolveFormat(op string, party rm.Party, t Type) (formatWrite, error) {
	f := c.format
	if !f.valid() {
		return formatWrite{}, fmt.Errorf("%s: %w: unknown party format %s", op, transport.ErrInvalidConfig, f)
	}
	body, err := f.marshal(party)
	if err != nil {
		return formatWrite{}, fmt.Errorf("%s: %w", op, &EncodeError{Format: f, Err: err})
	}
	fw := formatWrite{
		body:        body,
		contentType: f.MediaType(),
		accept:      mediaTypeJSON, // the Identifier body is JSON
		decode: func(body []byte) (rm.Party, error) {
			party, err := f.decodeParty(body, t)
			if err != nil {
				return nil, fmt.Errorf("demographic: decode %s PARTY body: %w", f, err)
			}
			return party, nil
		},
	}
	if c.Prefer == transport.PreferRepresentation {
		fw.accept = f.MediaType()
	}
	return fw, nil
}

// getConfig is the resolved option set for [Get] and the versioned_party
// reads.
type getConfig struct {
	format   Format
	raw      *Raw
	rawGiven bool // WithRawBody was given, so a nil raw is refused
}

// newGetConfig applies opts and refuses, before any request, the values a
// read cannot send: a format outside the two, and a nil WithRawBody
// destination. op names the read in the error.
func newGetConfig(op string, opts []GetOption) (getConfig, error) {
	var cfg getConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if !cfg.format.valid() {
		return getConfig{}, fmt.Errorf("%s: %w: unknown party format %s", op, transport.ErrInvalidConfig, cfg.format)
	}
	if cfg.rawGiven && cfg.raw == nil {
		return getConfig{}, fmt.Errorf("%s: %w: nil WithRawBody destination", op, transport.ErrInvalidConfig)
	}
	return cfg, nil
}

// do sends req and returns the response with its version metadata, which
// also comes back beside an error when a response arrived. On a 200 it
// copies the response's Content-Type and body into the WithRawBody
// destination, before any decode.
func (c getConfig) do(ctx context.Context, client *transport.Client, req *transport.Request) (*transport.Response, *openehrclient.VersionMetadata, error) {
	resp, err := client.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	if c.raw != nil && resp.StatusCode == http.StatusOK {
		*c.raw = Raw{ContentType: resp.Header.Get("Content-Type"), Body: resp.Body}
	}
	return resp, openehrclient.NewVersionMetadata(resp.Metadata), nil
}

// GetOption shapes the request a read sends and how it decodes the response.
// [Get] and the versioned_party reads ([GetVersionedParty],
// [GetRevisionHistory], [GetVersion], [GetVersionAtTime] and
// [GetVersionByID]) all take it.
type GetOption func(*getConfig)

// WithGetFormat sets the representation a read asks for and decodes. The
// default is [FormatJSON]. When the option is given more than once, the last
// one wins.
//
// The read sends f's media type as `Accept` and decodes a 200 body in f,
// whatever `Content-Type` the server labels it with. The body is decoded
// as the class the read names: the [Type] given to [Get], or the
// VERSIONED_PARTY, REVISION_HISTORY or ORIGINAL_VERSION a versioned_party
// read returns. A body that names another class does not decode: by its
// `_type`, or in canonical XML by its root element's `xsi:type` or, with
// none, by the element's name. The PARTY inside an ORIGINAL_VERSION is decoded by the
// `xsi:type` of its `data` element, as canonical JSON decodes it by `_type`.
//
// The read fails with [transport.ErrInvalidConfig], before any request,
// when f is neither [FormatJSON] nor [FormatXML].
func WithGetFormat(f Format) GetOption {
	return func(c *getConfig) { c.format = f }
}

// WithRawBody makes a read copy the response's `Content-Type` value and its
// body bytes into dst when the server answers 200 and the body was read,
// before any decode. The read fills dst even when it then fails: with
// [transport.ErrInvalidShape] for an empty or null body, or with a
// [*transport.DecodeError] for a body that does not decode. Every other
// outcome leaves dst as it was: a status other than 200, such as a 204 or an
// error status, a failure before any response arrives, or a body that could
// not be read. It works with both formats, the default included.
//
// A nil dst fails the read with [transport.ErrInvalidConfig] before any
// request.
func WithRawBody(dst *Raw) GetOption {
	return func(c *getConfig) {
		c.raw = dst
		c.rawGiven = true
	}
}

// Raw is what the server sent for a read, as received. A read fills it when
// [WithRawBody] is given.
type Raw struct {
	// ContentType is the response's `Content-Type` header value, parameters
	// included. It is empty when the server sent none.
	ContentType string
	// Body is the response body. It is the same slice a
	// [*transport.DecodeError] from the same call holds, not a copy. For a
	// party it is personal data.
	Body []byte
}
