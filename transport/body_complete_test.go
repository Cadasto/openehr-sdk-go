package transport_test

// body_complete_test.go — REQ-151 § Whether the body arrived whole.
//
// A *transport.BodyReadError says in Complete whether the server sent its
// whole answer before the body read failed. These tests drive transport.Client
// against a raw TCP listener that writes the response bytes by hand, so every
// read failure is the one net/http itself raises from bytes on the wire: a
// body cut mid-read, a malformed or over-long chunked trailer, a body that does
// not match its Content-Encoding, a body over the WithMaxResponseBody ceiling,
// and, over unencrypted HTTP/2, data past the declared Content-Length.
//
// Too many trailer lines has no case: net/http's client reads a response
// trailer with no limit on its lines, and the 4 KiB limit on a trailer's
// length stops a long one first, so that failure cannot reach a client.

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// rawServer listens on a loopback port and hands every connection to serve,
// which reads the request and writes the response bytes itself. The
// connection is closed when serve returns. It returns the server's base URL;
// the listener closes, and every serve call has returned, when the test ends.
func rawServer(t *testing.T, serve func(conn net.Conn)) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer func() { _ = conn.Close() }()
				// A backstop: a client that never closes cannot hold the
				// test past this.
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				serve(conn)
			})
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return "http://" + ln.Addr().String()
}

// newRawClient returns a transport.Client aimed at baseURL through tr. The
// transport's idle connections close when the test ends, before the server's
// cleanup waits for its connections.
func newRawClient(t *testing.T, baseURL string, tr *http.Transport, opts ...transport.Option) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(baseURL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.CloseIdleConnections)
	c, err := transport.New(cat, append([]transport.Option{transport.WithHTTPClient(&http.Client{Transport: tr})}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ehrGet is the request every case sends: a safe read, so nothing in the
// client's own policy depends on the method.
func ehrGet() *transport.Request {
	return &transport.Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"}
}

// gzipBytes compresses s.
func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, s); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rawResponse is a response head with a Content-Length for body, then body.
func rawResponse(header string, body []byte) []byte {
	head := "HTTP/1.1 200 OK\r\n" + header + "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	return append([]byte(head), body...)
}

// assertCompleteKeepsText checks that Complete changes neither the error's
// text nor what it wraps.
func assertCompleteKeepsText(t *testing.T, bre *transport.BodyReadError) {
	t.Helper()
	flipped := *bre
	flipped.Complete = !bre.Complete
	if got, want := flipped.Error(), bre.Error(); got != want {
		t.Errorf("BodyReadError.Error() with Complete=%v = %q, want the same text as with Complete=%v, %q", flipped.Complete, got, bre.Complete, want)
	}
	if got, want := flipped.Unwrap(), bre.Unwrap(); !errors.Is(got, want) {
		t.Errorf("BodyReadError.Unwrap() with Complete=%v = %v, want the same error as with Complete=%v, %v", flipped.Complete, got, bre.Complete, want)
	}
}

// TestBodyReadErrorCompleteHTTP1 pins Complete on every read failure an HTTP/1
// response can give: true where the server sent its whole answer (a body over
// the ceiling, a body that does not match its Content-Encoding, a failure in
// the chunked trailer after the body's last byte), false where the body
// stopped before its last byte, and false for a failure the client does not
// recognise.
func TestBodyReadErrorCompleteHTTP1(t *testing.T) { // REQ-151
	gz := gzipBytes(t, `{"_type":"COMPOSITION"}`)
	badCRC := bytes.Clone(gz)
	// The gzip trailer is the CRC-32 of the data, then its length: flipping
	// the CRC's first byte leaves the stream readable to its end.
	badCRC[len(badCRC)-8] ^= 0xff

	// A valid gzip header, then a deflate block of the reserved type 3.
	corruptDeflate := append(bytes.Clone(gz[:10]), 0x07, 0x00, 0x00, 0x00)

	cases := []struct {
		name string
		// response is written after the request is read; the connection
		// closes after it.
		response []byte
		opts     []transport.Option
		// premise holds for the read failure this case is meant to raise.
		premise func(inner error) bool
		want    bool
	}{
		{
			name:     "body cut before its last byte",
			response: []byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n{\"_type\":\"COMPOSITION\""),
			premise:  func(inner error) bool { return errors.Is(inner, io.ErrUnexpectedEOF) },
			want:     false,
		},
		{
			name:     "chunked body cut inside a chunk",
			response: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n10\r\nhello"),
			premise:  func(inner error) bool { return errors.Is(inner, io.ErrUnexpectedEOF) },
			want:     false,
		},
		{
			// A failure the client does not recognise: a chunk length that
			// is not hexadecimal, before the body's last byte.
			name:     "malformed chunk length",
			response: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\nzz\r\n"),
			premise:  func(inner error) bool { return inner != nil && !errors.Is(inner, io.ErrUnexpectedEOF) },
			want:     false,
		},
		{
			name:     "malformed chunked trailer",
			response: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\nno colon here\r\n\r\n"),
			premise: func(inner error) bool {
				_, ok := errors.AsType[textproto.ProtocolError](inner)
				return ok
			},
			want: true,
		},
		{
			// net/http looks for the trailer's end within its read buffer,
			// 4 KiB by default; this trailer line is twice that.
			name:     "over-long chunked trailer",
			response: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\nX-Pad: " + strings.Repeat("a", 8<<10) + "\r\n\r\n"),
			premise:  func(inner error) bool { return inner != nil && strings.Contains(inner.Error(), "trailer") },
			want:     true,
		},
		{
			name:     "gzip body that fails its checksum",
			response: rawResponse("Content-Encoding: gzip\r\n", badCRC),
			premise:  func(inner error) bool { return errors.Is(inner, gzip.ErrChecksum) },
			want:     true,
		},
		{
			name:     "plain body labelled gzip",
			response: rawResponse("Content-Encoding: gzip\r\n", []byte("this body is plain text, not gzip at all")),
			premise:  func(inner error) bool { return errors.Is(inner, gzip.ErrHeader) },
			want:     true,
		},
		{
			name:     "gzip body with a corrupt compressed stream",
			response: rawResponse("Content-Encoding: gzip\r\n", corruptDeflate),
			premise: func(inner error) bool {
				_, ok := errors.AsType[flate.CorruptInputError](inner)
				return ok
			},
			want: true,
		},
		{
			name:     "body over the ceiling",
			response: rawResponse("", bytes.Repeat([]byte("x"), 64)),
			opts:     []transport.Option{transport.WithMaxResponseBody(16)},
			premise:  func(inner error) bool { return errors.Is(inner, transport.ErrResponseTooLarge) },
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var acceptEncoding atomic.Value
			base := rawServer(t, func(conn net.Conn) {
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					t.Errorf("server: read request: %v", err)
					return
				}
				acceptEncoding.Store(req.Header.Get("Accept-Encoding"))
				_, _ = conn.Write(tc.response)
			})
			c := newRawClient(t, base, &http.Transport{}, tc.opts...)

			resp, err := c.Do(t.Context(), ehrGet())
			bre, ok := errors.AsType[*transport.BodyReadError](err)
			if !ok {
				t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v); the premise of this test is gone", err, err)
			}
			if !tc.premise(bre.Inner) {
				t.Fatalf("BodyReadError.Inner = %T (%v), not the read failure this case raises; the premise of this test is gone", bre.Inner, bre.Inner)
			}
			// net/http decompresses only a response to a request it added
			// Accept-Encoding: gzip to itself.
			if got, _ := acceptEncoding.Load().(string); !strings.Contains(got, "gzip") {
				t.Fatalf("the request carried Accept-Encoding %q, want gzip added by net/http; the premise of this test is gone", got)
			}
			assertTwoXXResponseKept(t, resp, http.StatusOK)

			if bre.Complete != tc.want {
				t.Errorf("BodyReadError.Complete = %v for read failure %T (%v), want %v", bre.Complete, bre.Inner, bre.Inner, tc.want)
			}
			assertCompleteKeepsText(t, bre)
		})
	}
}

// The HTTP/2 framing the h2c server below writes and reads (RFC 9113).
const (
	h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

	h2FrameData     = 0x0
	h2FrameHeaders  = 0x1
	h2FrameSettings = 0x4

	h2FlagEndStream  = 0x1
	h2FlagAck        = 0x1
	h2FlagEndHeaders = 0x4
)

// writeH2Frame writes one HTTP/2 frame: a 9-byte header (length, type, flags,
// stream identifier), then payload.
func writeH2Frame(w io.Writer, typ, flags byte, stream uint32, payload []byte) error {
	frame := make([]byte, 9, 9+len(payload))
	frame[0], frame[1], frame[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
	frame[3], frame[4] = typ, flags
	binary.BigEndian.PutUint32(frame[5:], stream&0x7fffffff)
	_, err := w.Write(append(frame, payload...))
	return err
}

// h2ResponseHeaders is the HPACK header block for status 200, the given
// Content-Length and, when contentEncoding is not empty, that
// Content-Encoding, without Huffman coding: the static table's ":status: 200"
// entry (index 8), then content-length (index 28) and content-encoding
// (index 26) as literals that are not indexed.
func h2ResponseHeaders(contentLength, contentEncoding string) []byte {
	block := []byte{0x80 | 8, 0x0f, 28 - 15, byte(len(contentLength))}
	block = append(block, contentLength...)
	if contentEncoding != "" {
		block = append(block, 0x0f, 26-15, byte(len(contentEncoding)))
		block = append(block, contentEncoding...)
	}
	return block
}

// h2cServer answers the first request on each connection, over HTTP/2 with
// prior knowledge, with status 200, the declared contentLength and
// contentEncoding, and one DATA frame of data that ends the stream. It sets answered once it has written
// the response, and then reads frames until the client closes the connection.
func h2cServer(t *testing.T, contentLength, contentEncoding, data string, answered *atomic.Bool) func(conn net.Conn) {
	return func(conn net.Conn) {
		br := bufio.NewReader(conn)
		preface := make([]byte, len(h2Preface))
		if _, err := io.ReadFull(br, preface); err != nil || string(preface) != h2Preface {
			t.Errorf("server: read HTTP/2 preface = (%q, %v), want %q", preface, err, h2Preface)
			return
		}
		if err := writeH2Frame(conn, h2FrameSettings, 0, 0, nil); err != nil {
			t.Errorf("server: write SETTINGS: %v", err)
			return
		}
		replied := false
		for {
			var hdr [9]byte
			if _, err := io.ReadFull(br, hdr[:]); err != nil {
				return
			}
			length := int64(hdr[0])<<16 | int64(hdr[1])<<8 | int64(hdr[2])
			typ, flags := hdr[3], hdr[4]
			stream := binary.BigEndian.Uint32(hdr[5:]) & 0x7fffffff
			if _, err := io.CopyN(io.Discard, br, length); err != nil {
				return
			}
			switch {
			case typ == h2FrameSettings && flags&h2FlagAck == 0:
				if err := writeH2Frame(conn, h2FrameSettings, h2FlagAck, 0, nil); err != nil {
					return
				}
			case typ == h2FrameHeaders && !replied:
				replied = true
				if err := writeH2Frame(conn, h2FrameHeaders, h2FlagEndHeaders, stream, h2ResponseHeaders(contentLength, contentEncoding)); err != nil {
					t.Errorf("server: write HEADERS: %v", err)
					return
				}
				if err := writeH2Frame(conn, h2FrameData, h2FlagEndStream, stream, []byte(data)); err != nil {
					t.Errorf("server: write DATA: %v", err)
					return
				}
				answered.Store(true)
			}
		}
	}
}

// TestBodyReadErrorCompleteHTTP2 pins Complete on HTTP/2, where a server can
// send more data than the Content-Length it declared: net/http reads the
// declared bytes and then fails, after the body's last byte, so the answer
// is whole. A gzip body runs past its Content-Length the same way, but
// net/http decompresses it and reports no Content-Length, so the declared
// length cannot tell it apart. A stream that ends short of its Content-Length
// is the control: the body stopped before its last byte.
func TestBodyReadErrorCompleteHTTP2(t *testing.T) { // REQ-151
	zipped := string(gzipBytes(t, "hello"))
	cases := []struct {
		name            string
		contentLength   string
		contentEncoding string
		data            string
		want            bool
	}{
		{name: "data past the declared Content-Length", contentLength: "5", data: "helloEXTRA", want: true},
		{name: "gzip data past the declared Content-Length", contentLength: strconv.Itoa(len(zipped)), contentEncoding: "gzip", data: zipped + "EXTRA", want: true},
		{name: "stream ends before the declared Content-Length", contentLength: "10", data: "hello", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var answered atomic.Bool
			base := rawServer(t, h2cServer(t, tc.contentLength, tc.contentEncoding, tc.data, &answered))
			// Unencrypted HTTP/2 alone: the client speaks HTTP/2 with prior
			// knowledge to an http:// URL.
			tr := &http.Transport{Protocols: new(http.Protocols)}
			tr.Protocols.SetUnencryptedHTTP2(true)
			c := newRawClient(t, base, tr)

			resp, err := c.Do(t.Context(), ehrGet())
			if !answered.Load() {
				t.Fatalf("the server never answered an HTTP/2 request (Do: %v); the premise of this test is gone", err)
			}
			bre, ok := errors.AsType[*transport.BodyReadError](err)
			if !ok {
				t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v); the premise of this test is gone", err, err)
			}
			assertTwoXXResponseKept(t, resp, http.StatusOK)

			if bre.Complete != tc.want {
				t.Errorf("BodyReadError.Complete = %v for read failure %T (%v), want %v", bre.Complete, bre.Inner, bre.Inner, tc.want)
			}
			assertCompleteKeepsText(t, bre)
		})
	}
}
