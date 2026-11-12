// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package aggrega

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

// Wire tests for the aggrega_pack_source and aggrega_read_source_file calls,
// driven against the shared payload vectors the ABI package publishes rather
// than against bytes produced here. That is what makes them discriminating: a kit
// that built its request with the wrong encoder, or read its reply with the wrong
// decoder, produces bytes that do not match a vector an independent instrument
// has already anchored — whereas a kit checked only against its own output
// round-trips cleanly however wrong both halves are, because the same mistake is
// on both sides.
//
// The vector readers live in parse_source_wire_test.go, in the same package: one
// resolver and one hex decoder serve every wire test here, so a second copy
// cannot drift from the first. That file also records why -count=1 matters when
// the vectors are what changed — they sit above this module's root, which the Go
// build cache does not track.

// TestEncodePackSourceRequest_MatchesSharedVector proves the request the kit
// builds is byte-identical to the shared SourcePackRequest vector, for the very
// field values that vector's companion document declares. A kit still building a
// text request fails here, because a text document and the canonical binary
// encoding of the same value are different bytes.
func TestEncodePackSourceRequest_MatchesSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Op   string `json:"op"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(readVector(t, "SourcePackRequest.golden.json"), &golden); err != nil {
		t.Fatalf("SourcePackRequest.golden.json is not readable as a SourcePackRequest: %v", err)
	}

	got, err := encodePackSourceRequest(golden.Op, golden.Path)
	if err != nil {
		t.Fatalf("encodePackSourceRequest: %v", err)
	}
	want := readHexVector(t, "SourcePackRequest.cbor.hex")
	if !bytes.Equal(got, want) {
		t.Errorf("request bytes = %x\nwant                 %x", got, want)
	}
}

// TestEncodeReadSourceFileRequest_MatchesSharedVector is the content-read sibling
// of the pack-source request assertion.
func TestEncodeReadSourceFileRequest_MatchesSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(readVector(t, "SourceFileRequest.golden.json"), &golden); err != nil {
		t.Fatalf("SourceFileRequest.golden.json is not readable as a SourceFileRequest: %v", err)
	}

	got, err := encodeReadSourceFileRequest(golden.Path)
	if err != nil {
		t.Fatalf("encodeReadSourceFileRequest: %v", err)
	}
	want := readHexVector(t, "SourceFileRequest.cbor.hex")
	if !bytes.Equal(got, want) {
		t.Errorf("request bytes = %x\nwant                 %x", got, want)
	}
}

// TestDecodePackSourceReply_SuccessFromSharedVector proves the kit reads the
// success shape out of the shared SourcePackResponse vector — the shape every
// production provider guest takes, and the only one carrying the packed files.
//
// This vector carries the projection with both failure fields omitted; the
// failure test below carries the failure fields with the projection omitted. Both
// directions are asserted because each hides the other's defect: a decoder that
// only reads the failure fields passes any test built from failure replies alone,
// and one that only reads the projection passes any test built from success
// replies alone.
func TestDecodePackSourceReply_SuccessFromSharedVector(t *testing.T) {
	t.Parallel()

	// The companion document carries the field values in the textual encoding,
	// where each file's content is base64. Read them from there rather than
	// restating them, so the two vectors cannot drift apart from this test.
	var golden struct {
		SourceProjection struct {
			Files []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"files"`
		} `json:"source_projection"`
	}
	if err := json.Unmarshal(readVector(t, "SourcePackResponse.golden.json"), &golden); err != nil {
		t.Fatalf("SourcePackResponse.golden.json is not readable as a SourcePackResponse: %v", err)
	}

	proj, err := decodePackSourceReply(readHexVector(t, "SourcePackResponse.cbor.hex"))
	if err != nil {
		t.Fatalf("decodePackSourceReply on the success shape: %v", err)
	}
	if len(proj.Files) != len(golden.SourceProjection.Files) {
		t.Fatalf("decoded %d files; the vector declares %d", len(proj.Files), len(golden.SourceProjection.Files))
	}
	for i, want := range golden.SourceProjection.Files {
		wantContent, err := base64.StdEncoding.DecodeString(want.Content)
		if err != nil {
			t.Fatalf("vector file %d content is not base64: %v", i, err)
		}
		if proj.Files[i].Path != want.Path {
			t.Errorf("file %d path = %q; want the vector's %q", i, proj.Files[i].Path, want.Path)
		}
		if !bytes.Equal(proj.Files[i].Content, wantContent) {
			t.Errorf("file %d content = %x; want the vector's %x", i, proj.Files[i].Content, wantContent)
		}
	}
}

// TestDecodePackSourceReply_FailureFromSharedVector proves the kit reads a
// reported failure out of the shared error-envelope vector and surfaces it as a
// HostError carrying both the machine-matchable code and the message.
func TestDecodePackSourceReply_FailureFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Error             string `json:"error"`
		HostfuncErrorCode string `json:"hostfunc_error_code"`
	}
	if err := json.Unmarshal(readVector(t, "ErrorEnvelopeSourcePackResponse.golden.json"), &golden); err != nil {
		t.Fatalf("ErrorEnvelopeSourcePackResponse.golden.json is not readable as a SourcePackResponse: %v", err)
	}

	proj, err := decodePackSourceReply(readHexVector(t, "ErrorEnvelopeSourcePackResponse.cbor.hex"))
	if err == nil {
		t.Fatalf("a failure reply decoded as a success carrying projection %+v", proj)
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("decodePackSourceReply returned %T (%v); want a *HostError", err, err)
	}
	if he.Code != golden.HostfuncErrorCode {
		t.Errorf("HostError code = %q; want the vector's %q", he.Code, golden.HostfuncErrorCode)
	}
	if he.Message != golden.Error {
		t.Errorf("HostError message = %q; want the vector's %q", he.Message, golden.Error)
	}
}

// TestDecodeReadSourceFileReply_SuccessFromSharedVector proves the kit reads the
// file's bytes out of the shared SourceFileResponse vector, whose companion
// document carries them base64 in the textual encoding.
func TestDecodeReadSourceFileReply_SuccessFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(readVector(t, "SourceFileResponse.golden.json"), &golden); err != nil {
		t.Fatalf("SourceFileResponse.golden.json is not readable as a SourceFileResponse: %v", err)
	}
	want, err := base64.StdEncoding.DecodeString(golden.Content)
	if err != nil {
		t.Fatalf("SourceFileResponse.golden.json content is not base64: %v", err)
	}

	got, err := decodeReadSourceFileReply(readHexVector(t, "SourceFileResponse.cbor.hex"))
	if err != nil {
		t.Fatalf("decodeReadSourceFileReply on the success shape: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("content = %q; want the vector's %q", got, want)
	}
}

// TestDecodeReadSourceFileReply_FailureFromSharedVector proves the content-read
// failure shape surfaces as a HostError, the other direction of the omission
// property from the success vector above.
func TestDecodeReadSourceFileReply_FailureFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Error             string `json:"error"`
		HostfuncErrorCode string `json:"hostfunc_error_code"`
	}
	if err := json.Unmarshal(readVector(t, "ErrorEnvelopeSourceFileResponse.golden.json"), &golden); err != nil {
		t.Fatalf("ErrorEnvelopeSourceFileResponse.golden.json is not readable as a SourceFileResponse: %v", err)
	}

	content, err := decodeReadSourceFileReply(readHexVector(t, "ErrorEnvelopeSourceFileResponse.cbor.hex"))
	if err == nil {
		t.Fatalf("a failure reply decoded as a success carrying content %q", content)
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("decodeReadSourceFileReply returned %T (%v); want a *HostError", err, err)
	}
	if he.Code != golden.HostfuncErrorCode {
		t.Errorf("HostError code = %q; want the vector's %q", he.Code, golden.HostfuncErrorCode)
	}
	if he.Message != golden.Error {
		t.Errorf("HostError message = %q; want the vector's %q", he.Message, golden.Error)
	}
}

// TestDecodeSourceReplies_RefuseTheOtherEncoding proves neither reply decoder is
// silently tolerant of a textual reply. Without this, a host left on the other
// encoding would be reported as some downstream shape error rather than as an
// undecodable reply, and the two are not the same diagnosis.
func TestDecodeSourceReplies_RefuseTheOtherEncoding(t *testing.T) {
	t.Parallel()

	if _, err := decodePackSourceReply([]byte(`{"source_projection":{"files":[]}}`)); err == nil {
		t.Errorf("a textual pack-source reply decoded without error; the reply slot carries the binary encoding only")
	}
	if _, err := decodeReadSourceFileReply([]byte(`{"content":"aGk="}`)); err == nil {
		t.Errorf("a textual read-source-file reply decoded without error; the reply slot carries the binary encoding only")
	}
}

// TestDecodePackSourceReply_EmptyReplyIsAnError proves a pack-source envelope
// carrying neither a projection nor a failure is reported rather than passing as
// an empty pack, so "the host said nothing" and "the selection matched nothing"
// stay distinguishable.
func TestDecodePackSourceReply_EmptyReplyIsAnError(t *testing.T) {
	t.Parallel()

	// a0: map, 0 pairs — every field omitted.
	if _, err := decodePackSourceReply([]byte{0xa0}); err == nil {
		t.Fatalf("an empty envelope decoded as a success")
	}
}

// TestDecodeReadSourceFileReply_EmptyReplyIsTheEmptyFile pins the deliberate
// asymmetry with the pack-source decoder: an empty content field is a legitimate
// read of an empty file, and the field is omitted when empty, so a reply with no
// failure and no content yields empty bytes rather than an error.
//
// The distinction is worth pinning rather than inferring: making this an error
// would refuse every empty file in an acquired source, and making the pack-source
// case lenient would let a silent host produce an empty pack.
func TestDecodeReadSourceFileReply_EmptyReplyIsTheEmptyFile(t *testing.T) {
	t.Parallel()

	// a0: map, 0 pairs — every field omitted.
	got, err := decodeReadSourceFileReply([]byte{0xa0})
	if err != nil {
		t.Fatalf("an empty-content reply must read as the empty file, not as an error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("content = %q; want the empty file", got)
	}
}

// TestSourceVectorHeads reads the map head byte of each shared vector this file
// drives, from the canonical encoding's own rules rather than from the encoder
// that wrote them. A head disagreeing with its body round-trips cleanly through
// the generator's own two halves, so reading the head is what a same-generator
// comparison cannot do.
//
// The heads assert the omission property in both directions at once: a success
// envelope carries only its payload key, and a failure envelope carries only the
// two failure keys.
func TestSourceVectorHeads(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		vector string
		want   byte
		why    string
	}{
		{"SourcePackRequest.cbor.hex", 0xa2, "two required fields: op and path"},
		{"SourceFileRequest.cbor.hex", 0xa1, "one required field: path"},
		{"SourcePackResponse.cbor.hex", 0xa1, "the projection alone; both failure fields omitted"},
		{"SourceFileResponse.cbor.hex", 0xa1, "the content alone; both failure fields omitted"},
		{"ErrorEnvelopeSourcePackResponse.cbor.hex", 0xa2, "message and code; the projection omitted"},
		{"ErrorEnvelopeSourceFileResponse.cbor.hex", 0xa2, "message and code; the content omitted"},
	} {
		b := readHexVector(t, tc.vector)
		if len(b) == 0 {
			t.Errorf("%s is empty", tc.vector)
			continue
		}
		if b[0] != tc.want {
			t.Errorf("%s head = %#x; want %#x (%s)", tc.vector, b[0], tc.want, tc.why)
		}
	}
}
