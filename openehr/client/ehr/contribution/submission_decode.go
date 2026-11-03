package contribution

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// UnmarshalSubmission decodes a canonical-JSON `Contribution_create` body, a
// contribution authored elsewhere as one document, into a [Submission] that
// [Commit] sends as the batch it describes. It is the inverse of
// [Submission.MarshalJSON].
//
// The batch `audit` and each version's `commit_audit` decode as an
// [UpdateAudit] from a `_type` of AUDIT_DETAILS or UPDATE_AUDIT, or from none,
// and keep the form they were read with in [UpdateAudit.Type], so an audit
// read without a `_type` encodes as the default AUDIT_DETAILS. A
// `time_committed` in an audit is dropped: the server assigns it.
//
// Each element of `versions` decodes as an *[OriginalVersion][T] when its
// `_type` is ORIGINAL_VERSION or absent, and as an *[ImportedVersion][T] when
// it is IMPORTED_VERSION. T is the type the `_type` of its `data` names (of
// `item.data` for an IMPORTED_VERSION): [rm.Composition], [rm.EHRStatus],
// [rm.Folder] or [rm.EHRAccess] for COMPOSITION, EHR_STATUS, FOLDER or
// EHR_ACCESS. An element with no `data`, or a null one, such as a [Deletion]
// built without a payload, decodes with a nil payload, and as the
// rm.Composition instantiation, since the type changes nothing on the wire
// when there is no payload. A top-level `_type`, which some producers write,
// is ignored.
//
// The decoded submission passes [Submission.Validate]; when it does not,
// UnmarshalSubmission returns Validate's error. MarshalJSON writes the result
// as it writes a submission built by hand.
//
// Every other failure the input causes is a *[canjson.DecodeError] whose Path
// is the JSON Pointer of the offending value from the root of the body, and
// whose text carries no value from the body: malformed JSON; a body that is
// not a JSON object (Path ""); an absent, null or empty `versions`
// ("/versions"); a version `_type` other than the two ("/versions/0/_type");
// an audit `_type` other than the three ("/audit/_type",
// "/versions/0/commit_audit/_type"); an IMPORTED_VERSION with no `item`, or
// a null one ("/versions/0/item"); a `data` without a `_type`
// ("/versions/0/data") or with one outside the four
// ("/versions/0/data/_type"); and a failure inside a payload or an audit, such
// as an unknown `_type` within a composition, which canjson reports at its
// pointer from the root of the body ("/versions/0/data/content/0"). A `_type`
// the RM defines but the body cannot hold there wraps
// [typereg.ErrTypeMismatch], one it does not define wraps
// [typereg.ErrUnknownType], a missing payload `_type` wraps
// [typereg.ErrMissingType], and a value of the wrong JSON kind wraps
// [canjson.ErrInvalidShape].
//
// opts are canjson's decode options. [canjson.WithUnmatched],
// [canjson.WithStrict] and [canjson.WithMissing] check the members of the
// audits and the versions, payloads included, as [canjson.Unmarshal] checks
// an RM value, and their callbacks receive JSON Pointers from the root of the
// body. The audits are checked as AUDIT_DETAILS, the RM class whose attributes
// the write-side audit shares. A member the write-side shape leaves out by
// design is never missing: `time_committed` and the optional `system_id` on
// the batch audit and on each commit_audit, and `uid` and `contribution` on
// each version. Two things
// are not checked: the members of the body itself beside `audit` and
// `versions`, since the body is a REST request shape rather than an RM class,
// and the payload of an IMPORTED_VERSION's item, which [ImportedVersion] holds
// as plain JSON. The check runs once the submission has decoded and passed
// Validate, so a decode that fails reports nothing, and a decode error comes
// back as itself under [canjson.WithStrict]. Checking decodes each audit and
// version a second time.
func UnmarshalSubmission(data []byte, opts ...canjson.DecoderOption) (*Submission, error) {
	body, err := readSubmissionBody(data)
	if err != nil {
		return nil, err
	}
	sub, err := body.decode()
	if err != nil {
		return nil, err
	}
	if err := sub.Validate(); err != nil {
		return nil, err
	}
	if len(opts) > 0 {
		if err := body.check(opts); err != nil {
			return nil, err
		}
	}
	return sub, nil
}

// The member names and discriminators a Contribution_create body carries.
const (
	memberType          = "_type"
	memberAudit         = "audit"
	memberVersions      = "versions"
	memberCommitAudit   = "commit_audit"
	memberData          = "data"
	memberItem          = "item"
	memberUID           = "uid"
	memberContribution  = "contribution"
	memberTimeCommitted = "time_committed"
	memberSystemID      = "system_id"

	typeOriginalVersion = "ORIGINAL_VERSION"
	typeImportedVersion = "IMPORTED_VERSION"
)

// The check pass writes these stand-ins where the write-side shape leaves a
// member out by design (REQ-130 § Audit, § Server-assigned fields), so the
// walk behind canjson.WithMissing finds the member present. Each carries
// every member the RM requires of its own class, so nothing inside it is
// reported either, and each is an attribute of the class it is written into,
// so it is never unmatched. Only the check pass sees them; the decoded value
// never does.
const (
	standInTimeCommitted = `{"_type":"DV_DATE_TIME","value":""}`
	standInSystemID      = `""`
	standInUID           = `{"_type":"OBJECT_VERSION_ID","value":""}`
	standInContribution  = `{"_type":"OBJECT_REF","id":{"_type":"HIER_OBJECT_ID","value":""},"namespace":"","type":""}`
)

// submissionBody is a Contribution_create body split into the pieces the
// decoder reads, each already checked for what the decoder needs to place it.
//
// Each piece is decoded, and checked, as a document of its own that places
// it where the body does: the batch audit as {"audit":…}, version i as
// {"versions":{"i":…}}, decoded into maps of the RM type. A JSON Pointer
// canjson reports into such a document, a decode failure's or a member an
// option reports, is the pointer into the body, since an object key "i" and
// an array index i are written alike. canjson.DecoderOption cannot be read or
// wrapped, so this is how the options' callbacks receive pointers from the
// body's root (REQ-130 § Decoding a submission, REQ-170, REQ-175).
type submissionBody struct {
	audit      *auditPiece // nil when the body carries no audit, or a null one
	auditFirst bool        // the audit comes before versions in the body
	versions   []*versionPiece
}

// readSubmissionBody splits data into the pieces of a submission, refusing
// what the decoder cannot place.
func readSubmissionBody(data []byte) (*submissionBody, error) {
	obj, isObject, err := readObject(data)
	if err != nil || !isObject {
		// Malformed JSON fails as canjson reports it, so the refusal is the
		// one every canonical-JSON decode gives (REQ-052).
		if verr := canjson.Unmarshal(data, new(jsontext.Value)); verr != nil {
			return nil, verr
		}
		if err != nil {
			return nil, &canjson.DecodeError{Inner: err}
		}
		return nil, refusal("", "", "the body is not a JSON object", canjson.ErrInvalidShape)
	}
	body := &submissionBody{}
	versions := -1
	for i, m := range obj.members {
		switch m.name {
		case memberAudit:
			body.auditFirst = versions < 0
			if body.audit, err = readAudit(obj.value(i), "/"+memberAudit); err != nil {
				return nil, err
			}
		case memberVersions:
			versions = i
		}
	}
	const at = "/" + memberVersions
	if versions < 0 {
		return nil, refusal(at, "", "the body has no versions", canjson.ErrInvalidShape)
	}
	elems, isArray, err := readArray(obj.value(versions))
	switch {
	case err != nil:
		return nil, nestedError(at, err)
	case !isArray:
		return nil, refusal(at, "", "versions is not a JSON array", canjson.ErrInvalidShape)
	case len(elems) == 0:
		return nil, refusal(at, "", "versions is empty", canjson.ErrInvalidShape)
	}
	body.versions = make([]*versionPiece, len(elems))
	for i, raw := range elems {
		if body.versions[i], err = readVersion(raw, i); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// decode decodes the pieces into a submission, in the order of the body.
func (b *submissionBody) decode() (*Submission, error) {
	sub := &Submission{Versions: make([]CommitVersion, len(b.versions))}
	if b.audit != nil {
		var d map[string]*rm.AuditDetails
		if err := canjson.Unmarshal(b.audit.document(false), &d); err != nil {
			return nil, err
		}
		sub.Audit = updateAuditFromLike(d[memberAudit])
		sub.Audit.Type = b.audit.typ
	}
	for i, v := range b.versions {
		var err error
		if sub.Versions[i], err = v.codec.decode(v.document(false), v.key(), v.auditType()); err != nil {
			return nil, err
		}
	}
	return sub, nil
}

// check applies opts to the members of each piece, in the order of the body.
func (b *submissionBody) check(opts []canjson.DecoderOption) error {
	checkAudit := func() error {
		if b.audit == nil {
			return nil
		}
		return checkDocument[map[string]*rm.AuditDetails](b.audit.document(true), opts)
	}
	if b.auditFirst {
		if err := checkAudit(); err != nil {
			return err
		}
	}
	for _, v := range b.versions {
		if err := v.codec.check(v.document(true), opts); err != nil {
			return err
		}
	}
	if !b.auditFirst {
		return checkAudit()
	}
	return nil
}

// checkDocument decodes doc into a new T with opts, for the members the
// options check; the value is dropped.
func checkDocument[T any](doc []byte, opts []canjson.DecoderOption) error {
	var target T
	return canjson.Unmarshal(doc, &target, opts...)
}

// auditPiece is a write-side audit as the body writes it: the batch audit or
// a version's commit_audit.
type auditPiece struct {
	obj        jsonObject
	typ        AuditType // the `_type` it was read with; "" when it has none
	typeMember int       // the index of its `_type` member, or -1
}

// readAudit reads the audit raw holds, at pointer at. It returns nil for a
// null audit, which decodes as an absent one.
func readAudit(raw []byte, at string) (*auditPiece, error) {
	if isNull(raw) {
		return nil, nil
	}
	obj, isObject, err := readObject(raw)
	switch {
	case err != nil:
		return nil, nestedError(at, err)
	case !isObject:
		return nil, refusal(at, "", "the audit is not a JSON object", canjson.ErrInvalidShape)
	}
	a := &auditPiece{obj: obj, typeMember: obj.index(memberType)}
	if a.typeMember < 0 {
		return a, nil
	}
	typ, isString := stringValue(obj.value(a.typeMember))
	switch {
	case !isString:
		return nil, refusal(at+"/"+memberType, "", "the audit's _type is not a string", canjson.ErrInvalidShape)
	case typ == "":
		// An empty `_type` reads as an omitted one, as the RM decoders read it.
	case typ == string(AuditTypeAuditDetails), typ == string(AuditTypeUpdateAudit):
		a.typ = AuditType(typ)
	default:
		return nil, typeRefusal(at+"/"+memberType, typ, "an audit's _type is AUDIT_DETAILS or UPDATE_AUDIT")
	}
	return a, nil
}

// bytes returns the audit as the decoder reads it, an AUDIT_DETAILS: an
// UPDATE_AUDIT `_type` is written as AUDIT_DETAILS, the RM class whose
// attributes the write-side audit shares. For the check pass (forCheck), a
// time_committed or system_id the audit leaves out or holds null is given its
// stand-in: the server assigns the first, and the write-side audit leaves the
// second optional.
func (a *auditPiece) bytes(forCheck bool) []byte {
	e := objectEdit{obj: a.obj}
	if a.typ == AuditTypeUpdateAudit {
		e.replace(a.typeMember, `"`+string(AuditTypeAuditDetails)+`"`)
	}
	if forCheck {
		e.supply(memberTimeCommitted, standInTimeCommitted)
		e.supply(memberSystemID, standInSystemID)
	}
	return e.bytes()
}

// document returns the batch audit placed where the body places it.
func (a *auditPiece) document(forCheck bool) []byte {
	return slices.Concat([]byte(`{"`+memberAudit+`":`), a.bytes(forCheck), []byte(`}`))
}

// versionPiece is one element of `versions` as the body writes it.
type versionPiece struct {
	index       int
	obj         jsonObject
	kind        string      // typeOriginalVersion or typeImportedVersion
	audit       *auditPiece // its commit_audit; nil when absent or null
	auditMember int         // the index of its commit_audit member, or -1
	codec       versionCodec
}

// readVersion reads the element raw holds at index i of `versions`.
func readVersion(raw []byte, i int) (*versionPiece, error) {
	at := "/" + memberVersions + "/" + strconv.Itoa(i)
	obj, isObject, err := readObject(raw)
	switch {
	case err != nil:
		return nil, nestedError(at, err)
	case !isObject:
		return nil, refusal(at, "", "a version is not a JSON object", canjson.ErrInvalidShape)
	}
	v := &versionPiece{index: i, obj: obj, kind: typeOriginalVersion, auditMember: obj.index(memberCommitAudit)}
	if t := obj.index(memberType); t >= 0 {
		typ, isString := stringValue(obj.value(t))
		switch {
		case !isString:
			return nil, refusal(at+"/"+memberType, "", "the version's _type is not a string", canjson.ErrInvalidShape)
		case typ == "", typ == typeOriginalVersion:
			// An element with no `_type` reads as an ORIGINAL_VERSION.
		case typ == typeImportedVersion:
			v.kind = typ
		default:
			return nil, typeRefusal(at+"/"+memberType, typ, "a version's _type is ORIGINAL_VERSION or IMPORTED_VERSION")
		}
	}
	if v.auditMember >= 0 {
		if v.audit, err = readAudit(obj.value(v.auditMember), at+"/"+memberCommitAudit); err != nil {
			return nil, err
		}
	}
	payloadAt, payload, err := v.payloadType(at)
	if err != nil {
		return nil, err
	}
	codec, ok := codecFor(v.kind, payload)
	if !ok {
		return nil, typeRefusal(payloadAt+"/"+memberType, payload, "a payload is a COMPOSITION, EHR_STATUS, FOLDER or EHR_ACCESS")
	}
	v.codec = codec
	return v, nil
}

// payloadType returns the pointer of the version's payload, at `data`, or
// `item.data` for an IMPORTED_VERSION, with the `_type` it carries, or "" when
// the version has none. at is the version's own pointer.
func (v *versionPiece) payloadType(at string) (string, string, error) {
	holder := v.obj
	if v.kind == typeImportedVersion {
		at += "/" + memberItem
		i := holder.index(memberItem)
		if i < 0 || isNull(holder.value(i)) {
			// The RM makes IMPORTED_VERSION.item mandatory, and the encoder
			// would write one the body never held.
			return "", "", refusal(at, "", "an IMPORTED_VERSION has no item", canjson.ErrInvalidShape)
		}
		item, isObject, err := readObject(holder.value(i))
		switch {
		case err != nil:
			return "", "", nestedError(at, err)
		case !isObject:
			return "", "", refusal(at, "", "the imported item is not a JSON object", canjson.ErrInvalidShape)
		}
		holder = item
	}
	at += "/" + memberData
	i := holder.index(memberData)
	if i < 0 || isNull(holder.value(i)) {
		return at, "", nil
	}
	data, isObject, err := readObject(holder.value(i))
	switch {
	case err != nil:
		return "", "", nestedError(at, err)
	case !isObject:
		return "", "", refusal(at, "", "the payload is not a JSON object", canjson.ErrInvalidShape)
	}
	t := data.index(memberType)
	if t < 0 {
		return "", "", refusal(at, "", "the payload has no _type", typereg.ErrMissingType)
	}
	typ, isString := stringValue(data.value(t))
	switch {
	case !isString:
		return "", "", refusal(at+"/"+memberType, "", "the payload's _type is not a string", canjson.ErrInvalidShape)
	case typ == "":
		return "", "", refusal(at, "", "the payload's _type is empty", typereg.ErrMissingType)
	}
	return at, typ, nil
}

// key is the version's index, the key that places it in its document.
func (v *versionPiece) key() string { return strconv.Itoa(v.index) }

// auditType is the `_type` the version's commit_audit was read with.
func (v *versionPiece) auditType() AuditType {
	if v.audit == nil {
		return ""
	}
	return v.audit.typ
}

// document returns the version placed where the body places it, its
// commit_audit read as an AUDIT_DETAILS (see [auditPiece.bytes]). For the
// check pass (forCheck), the server-assigned `contribution`, and `uid` on an
// ORIGINAL_VERSION, are given their stand-ins where the version leaves them
// out or holds null. An IMPORTED_VERSION has no `uid` of its own: it is its
// item's.
func (v *versionPiece) document(forCheck bool) []byte {
	e := objectEdit{obj: v.obj}
	if v.audit != nil && (forCheck || v.audit.typ == AuditTypeUpdateAudit) {
		e.replace(v.auditMember, string(v.audit.bytes(forCheck)))
	}
	if forCheck {
		if v.kind == typeOriginalVersion {
			e.supply(memberUID, standInUID)
		}
		e.supply(memberContribution, standInContribution)
	}
	return slices.Concat([]byte(`{"`+memberVersions+`":{"`+v.key()+`":`), e.bytes(), []byte(`}}`))
}

// versionCodec decodes and checks a versions[] element as one of the eight
// wrapper instantiations a submission admits, the element placed in its
// document (see [submissionBody]).
type versionCodec struct {
	decode func(doc []byte, key string, audit AuditType) (CommitVersion, error)
	check  func(doc []byte, opts []canjson.DecoderOption) error
}

// codecFor returns the codec of the wrapper a versions[] element of kind
// decodes as, for a payload whose `_type` is payload. A version with no
// payload ("") takes the rm.Composition instantiation: without a payload the
// type parameter changes nothing on the wire. It reports false for a payload
// outside the four versionable types. The eight instantiations are written out,
// as [Submission.Validate] lists them, rather than found by reflection.
func codecFor(kind, payload string) (versionCodec, bool) {
	imported := kind == typeImportedVersion
	switch payload {
	case "", "COMPOSITION":
		if imported {
			return importedCodec[rm.Composition](), true
		}
		return originalCodec[rm.Composition](), true
	case "EHR_STATUS":
		if imported {
			return importedCodec[rm.EHRStatus](), true
		}
		return originalCodec[rm.EHRStatus](), true
	case "FOLDER":
		if imported {
			return importedCodec[rm.Folder](), true
		}
		return originalCodec[rm.Folder](), true
	case "EHR_ACCESS":
		if imported {
			return importedCodec[rm.EHRAccess](), true
		}
		return originalCodec[rm.EHRAccess](), true
	}
	return versionCodec{}, false
}

func originalCodec[T any]() versionCodec {
	return versionCodec{decode: decodeOriginal[T], check: checkDocument[map[string]map[string]*rm.OriginalVersion[T]]}
}

func importedCodec[T any]() versionCodec {
	return versionCodec{decode: decodeImported[T], check: checkDocument[map[string]map[string]*rm.ImportedVersion[T]]}
}

// decodeOriginal decodes the ORIGINAL_VERSION placed at key in doc, its
// commit_audit kept with the `_type` it was read with.
func decodeOriginal[T any](doc []byte, key string, audit AuditType) (CommitVersion, error) {
	var d map[string]map[string]*rm.OriginalVersion[T]
	if err := canjson.Unmarshal(doc, &d); err != nil {
		return nil, err
	}
	w := WrapOriginalVersion(d[memberVersions][key])
	w.CommitAudit.Type = audit
	return w, nil
}

// decodeImported decodes the IMPORTED_VERSION placed at key in doc, its
// commit_audit kept with the `_type` it was read with.
func decodeImported[T any](doc []byte, key string, audit AuditType) (CommitVersion, error) {
	var d map[string]map[string]*rm.ImportedVersion[T]
	if err := canjson.Unmarshal(doc, &d); err != nil {
		return nil, err
	}
	w := WrapImportedVersion(d[memberVersions][key])
	w.CommitAudit.Type = audit
	return w, nil
}

// refusal is the error for a body the decoder cannot place: a
// *canjson.DecodeError at path whose cause says why and wraps sentinel, the
// class its text names. typ is the `_type` read there, if any.
func refusal(path, typ, why string, sentinel error) error {
	return &canjson.DecodeError{Path: path, Type: typ, Inner: fmt.Errorf("contribution: %s: %w", why, sentinel)}
}

// typeRefusal refuses the `_type` typ at path, a discriminator the body
// cannot hold there: as typereg.ErrTypeMismatch when typ names an RM type,
// and as typereg.ErrUnknownType when it names none, as a polymorphic slot
// refuses one.
func typeRefusal(path, typ, why string) error {
	sentinel := typereg.ErrUnknownType
	if _, known := typereg.Default.Lookup(typ); known {
		sentinel = typereg.ErrTypeMismatch
	}
	return refusal(path, typ, why, sentinel)
}

// nestedError reports err, a tokenizer refusal of a value inside the body at
// pointer at, as canjson reports one: a duplicate member name gains
// canjson.ErrInvalidShape. The body as a whole is read before its values are,
// so this is reached only where reading a value refuses what reading the
// body accepted.
func nestedError(at string, err error) error {
	path := at
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok && se != nil {
		path += string(se.JSONPointer)
	}
	return &canjson.DecodeError{Path: path, Inner: typereg.ClassifyDuplicate(err)}
}

// jsonObject is a JSON object as the document writes it: its bytes and the
// position of each member's value in them.
type jsonObject struct {
	raw     []byte
	members []jsonMember
}

// jsonMember is one member of a [jsonObject].
type jsonMember struct {
	name       string // unescaped
	start, end int    // the bounds of the value in the object's bytes
}

// index returns the index of the member named name, or -1.
func (o jsonObject) index(name string) int {
	return slices.IndexFunc(o.members, func(m jsonMember) bool { return m.name == name })
}

// value returns the bytes of the i-th member's value.
func (o jsonObject) value(i int) []byte {
	m := o.members[i]
	return o.raw[m.start:m.end]
}

// readObject reads raw, which holds one JSON value and nothing after it, as an
// object. It reports false, and no error, when the value is not an object.
func readObject(raw []byte) (jsonObject, bool, error) {
	// A bytes.Buffer is read in place; any other io.Reader is copied.
	dec := jsontext.NewDecoder(bytes.NewBuffer(raw))
	tok, err := dec.ReadToken()
	if err != nil {
		return jsonObject{}, false, err
	}
	if tok.Kind() != '{' {
		return jsonObject{}, false, nil
	}
	obj := jsonObject{raw: raw}
	for dec.PeekKind() == '"' {
		tok, err := dec.ReadToken()
		if err != nil {
			return jsonObject{}, false, err
		}
		// The token is valid only until the next read.
		name := tok.String()
		start := skipSeparators(raw, dec.InputOffset())
		if _, err := dec.ReadValue(); err != nil {
			return jsonObject{}, false, err
		}
		obj.members = append(obj.members, jsonMember{name: name, start: start, end: int(dec.InputOffset())})
	}
	if err := readEnd(dec); err != nil {
		return jsonObject{}, false, err
	}
	return obj, true, nil
}

// readArray reads raw, which holds one JSON value and nothing after it, as an
// array, returning its elements' bytes. It reports false, and no error, when
// the value is not an array.
func readArray(raw []byte) ([][]byte, bool, error) {
	dec := jsontext.NewDecoder(bytes.NewBuffer(raw))
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, false, err
	}
	if tok.Kind() != '[' {
		return nil, false, nil
	}
	var elems [][]byte
	for k := dec.PeekKind(); k != ']' && k != 0; k = dec.PeekKind() {
		start := skipSeparators(raw, dec.InputOffset())
		if _, err := dec.ReadValue(); err != nil {
			return nil, false, err
		}
		elems = append(elems, raw[start:dec.InputOffset()])
	}
	if err := readEnd(dec); err != nil {
		return nil, false, err
	}
	return elems, true, nil
}

// readEnd reads the token that closes the object or array dec is in, and
// then requires the end of the input.
func readEnd(dec *jsontext.Decoder) error {
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	switch _, err := dec.ReadToken(); {
	case err == io.EOF: //nolint:errorlint // jsontext returns io.EOF itself at the end of the input
		return nil
	case err != nil:
		return err
	}
	return errors.New("contribution: content after the JSON value")
}

// skipSeparators returns the offset of the first byte at or after off in raw
// that is not white space, a name separator or a value separator: where the
// value after them starts.
func skipSeparators(raw []byte, off int64) int {
	i := int(off)
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\n', '\r', ':', ',':
			i++
			continue
		}
		break
	}
	return i
}

// isNull reports whether raw is the JSON null.
func isNull(raw []byte) bool { return string(raw) == "null" }

// stringValue returns the string raw holds, and false when it holds no
// string.
func stringValue(raw []byte) (string, bool) {
	if jsontext.Value(raw).Kind() != '"' {
		return "", false
	}
	s, err := jsontext.AppendUnquote(nil, raw)
	return string(s), err == nil
}

// objectEdit rewrites a [jsonObject]: member values replaced, and members
// appended before its closing brace.
type objectEdit struct {
	obj      jsonObject
	replaced []replacement
	appended []string
}

// replacement is the new text of one member's value.
type replacement struct {
	member int
	text   string
}

// replace writes text as the value of the i-th member.
func (e *objectEdit) replace(i int, text string) {
	e.replaced = append(e.replaced, replacement{member: i, text: text})
}

// supply writes text as the value of the member named name when the object
// leaves it out or holds null there.
func (e *objectEdit) supply(name, text string) {
	i := e.obj.index(name)
	switch {
	case i < 0:
		e.appended = append(e.appended, strconv.Quote(name)+":"+text)
	case isNull(e.obj.value(i)):
		e.replace(i, text)
	}
}

// bytes returns the rewritten object: the object's own bytes when nothing was
// rewritten.
func (e *objectEdit) bytes() []byte {
	if len(e.replaced) == 0 && len(e.appended) == 0 {
		return e.obj.raw
	}
	raw := e.obj.raw
	slices.SortFunc(e.replaced, func(a, b replacement) int { return cmp.Compare(a.member, b.member) })
	out := make([]byte, 0, len(raw)+64)
	prev := 0
	for _, r := range e.replaced {
		m := e.obj.members[r.member]
		out = append(out, raw[prev:m.start]...)
		out = append(out, r.text...)
		prev = m.end
	}
	closing := bytes.LastIndexByte(raw, '}')
	out = append(out, raw[prev:closing]...)
	for i, m := range e.appended {
		if i > 0 || len(e.obj.members) > 0 {
			out = append(out, ',')
		}
		out = append(out, m...)
	}
	return append(out, raw[closing:]...)
}
