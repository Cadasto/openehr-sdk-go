package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/auth/basic"
	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// requestTimeout bounds one request to a CDR.
const requestTimeout = 2 * time.Minute

// maxBodyText is how much of a CDR's refusal text a record keeps.
const maxBodyText = 8000

// statusTag is the observer tag that carries a statusHolder to the
// observer, so each call learns the HTTP status it got whatever its outcome.
const statusTag = "cdrcheck.status"

// statusHolder receives the final HTTP status of one call.
type statusHolder struct{ code int }

// statusObserver copies each call's final status into the statusHolder its
// context carries. The transport calls it on the goroutine that issued the
// call, before the call returns.
type statusObserver struct{}

func (statusObserver) OnRequest(o transport.Observation) {
	if h, ok := o.Tags[statusTag].(*statusHolder); ok && h != nil {
		h.code = o.StatusCode
	}
}

// cdrClient is the run's connection to one CDR, through the SDK's own
// transport and its openEHR REST clients.
type cdrClient struct {
	name   string
	levels []definition.ExampleDetailLevel
	c      *transport.Client
}

// newCDRClient builds the client for t. It keeps the CDRs' error bodies
// (transport.WithRawErrorBodies): the refusal text is what the cross-check
// records, and it stays in the run's local output.
func newCDRClient(t cdrTarget, workers int) (*cdrClient, error) {
	u, err := url.Parse(t.base)
	if err != nil {
		return nil, fmt.Errorf("%s base URL does not parse", t.name)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: t.base,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: u, SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s catalog: %w", t.name, err)
	}
	rt := http.DefaultTransport.(*http.Transport).Clone()
	rt.MaxIdleConnsPerHost = workers
	opts := []transport.Option{
		transport.WithHTTPClient(&http.Client{Transport: rt, Timeout: requestTimeout}),
		transport.WithRawErrorBodies(true),
		transport.WithObserver(statusObserver{}),
	}
	if t.user != "" {
		src, err := basic.New(t.user, t.pass)
		if err != nil {
			return nil, fmt.Errorf("%s credential: %w", t.name, err)
		}
		opts = append(opts, transport.WithTokenSource(src))
	}
	c, err := transport.New(cat, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s client: %w", t.name, err)
	}
	return &cdrClient{name: t.name, levels: t.levels, c: c}, nil
}

// outcome is what one request to a CDR came to.
type outcome struct {
	Status int  `json:"status"`
	OK     bool `json:"ok"`
	// Body is the CDR's answer to a refused request: its error text.
	Body string `json:"body,omitempty"`
	// Error is a failure the CDR's status does not describe, such as a
	// connection refused or a 2xx answer the SDK could not read.
	Error string `json:"error,omitempty"`
}

// call runs fn with a context that collects its HTTP status, and turns its
// error into an outcome.
func call(ctx context.Context, fn func(ctx context.Context) error) outcome {
	h := &statusHolder{}
	err := fn(transport.WithObservationTag(ctx, statusTag, h))
	o := outcome{Status: h.code}
	if err == nil {
		o.OK = true
		return o
	}
	if we, ok := errors.AsType[*transport.WireError](err); ok && we != nil {
		o.Status = we.StatusCode
		o.Body = clip(string(we.RawBody))
		if o.Body == "" && we.OpenEHR != nil {
			o.Body = clip(we.OpenEHR.Message)
		}
		return o
	}
	o.Error = clip(err.Error())
	return o
}

// clip cuts s to maxBodyText bytes, on a rune boundary.
func clip(s string) string {
	if len(s) <= maxBodyText {
		return s
	}
	s = s[:maxBodyText]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

// refusedAsPresent reports whether an upload refusal says the CDR holds a
// template under that id already: a 409, or another 4xx whose text says so.
func refusedAsPresent(o outcome) bool {
	return o.Status == http.StatusConflict ||
		(o.Status >= 400 && o.Status < 500 && strings.Contains(strings.ToLower(o.Body), "already"))
}

// upload sends one OPT (no byte-order mark, see stripBOM) to the CDR.
func (c *cdrClient) upload(ctx context.Context, opt []byte) (outcome, string) {
	var stored string
	o := call(ctx, func(ctx context.Context) error {
		meta, _, err := definition.UploadTemplate(ctx, c.c, definition.FormatADL14, bytes.NewReader(stripBOM(opt)))
		if meta != nil {
			stored = meta.TemplateID
		}
		return err
	})
	return o, stored
}

// storedTemplate fetches the OPT the CDR holds under id.
func (c *cdrClient) storedTemplate(ctx context.Context, id string) ([]byte, outcome) {
	var body []byte
	o := call(ctx, func(ctx context.Context) error {
		b, _, err := definition.GetTemplate(ctx, c.c, id, definition.FormatADL14)
		body = b
		return err
	})
	return body, o
}

// newEHR creates an EHR with no request body, so the request carries no
// Content-Type (a CDR may answer 415 to one on an empty body). Every
// document the run posts goes to an EHR of its own: FerroEHR keeps one
// composition per EHR for a persistent template, and a document then never
// meets another's state.
//
// An EHR the CDR created but described in a body the SDK cannot read is
// still used: its id is the last segment of the Location header.
func (c *cdrClient) newEHR(ctx context.Context) (openehrclient.EHRID, outcome) {
	var id openehrclient.EHRID
	o := call(ctx, func(ctx context.Context) error {
		ehr, meta, err := openehrclient.Create(ctx, c.c)
		if ehr != nil && ehr.EHRID.Value != "" {
			id = openehrclient.EHRID(ehr.EHRID.Value)
			return err
		}
		if _, wire := errors.AsType[*transport.WireError](err); err != nil && !wire && meta != nil && meta.VersionUID != "" {
			id = openehrclient.EHRID(meta.VersionUID)
			return nil
		}
		return err
	})
	if id == "" && o.OK {
		o.OK, o.Error = false, "the CDR created an EHR but named no ehr_id"
	}
	return id, o
}

// postComposition posts comp, as the SDK serialises it, to a fresh EHR.
func (c *cdrClient) postComposition(ctx context.Context, comp *rm.Composition) (openehrclient.EHRID, outcome, bool) {
	id, eo := c.newEHR(ctx)
	if !eo.OK {
		return "", eo, false
	}
	return id, call(ctx, func(ctx context.Context) error {
		_, _, err := composition.Save(ctx, c.c, id, comp)
		return err
	}), true
}

// postRaw posts a canonical-JSON document as given to a fresh EHR. A CDR's
// own example goes this way, so that what the other CDR judges is that
// CDR's bytes, not the SDK's reading of them.
func (c *cdrClient) postRaw(ctx context.Context, doc []byte) (openehrclient.EHRID, outcome, bool) {
	id, eo := c.newEHR(ctx)
	if !eo.OK {
		return "", eo, false
	}
	return id, call(ctx, func(ctx context.Context) error {
		_, err := c.c.Do(ctx, &transport.Request{
			Method: http.MethodPost,
			Path:   "/ehr/" + string(id) + "/composition",
			Route:  "/ehr/{ehr_id}/composition",
			Body:   doc,
			Prefer: transport.PreferMinimal,
		})
		return err
	}), true
}

// example fetches the CDR's own example for template id, as the bytes the
// CDR sent. level "" sends no detail_level.
func (c *cdrClient) example(ctx context.Context, id string, level definition.ExampleDetailLevel) ([]byte, outcome) {
	var body []byte
	o := call(ctx, func(ctx context.Context) error {
		req := &transport.Request{
			Method: http.MethodGet,
			Path:   "/definition/template/" + definition.FormatADL14.PathSegment() + "/" + id + "/example",
			Route:  "/definition/template/{format}/{template_id}/example",
			Accept: "application/json",
		}
		if level != "" {
			req.Query = url.Values{"detail_level": []string{string(level)}}
		}
		resp, err := c.c.Do(ctx, req)
		if resp != nil {
			body = resp.Body
		}
		return err
	})
	return body, o
}

// withoutUID returns the JSON object doc with its top-level uid member
// removed and every other member, in order, as it was. A CDR's example can
// carry a uid, which a second CDR, or the first one again, would see twice.
func withoutUID(doc []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	var out bytes.Buffer
	out.WriteByte('{')
	first := true
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("object member without a name")
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		if key == "uid" {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		out.Write(k)
		out.WriteByte(':')
		out.Write(val)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}
