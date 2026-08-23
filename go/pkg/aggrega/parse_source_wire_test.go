// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Wire tests for the aggrega_parse_source call, driven against the shared
// payload vectors the ABI package publishes rather than against bytes produced
// here. That is what makes them discriminating: a kit that built its request
// with the wrong encoder, or read its reply with the wrong decoder, produces
// bytes that do not match a vector an independent instrument has already
// anchored — whereas a kit checked only against its own output round-trips
// cleanly however wrong both halves are, because the same mistake is on both
// sides.
//
// The vectors live in the sibling ABI checkout, above this module's root, which
// the Go build cache does not track: an edit to a vector does not by itself
// invalidate a cached result for this package. Run with -count=1 when the
// vectors are what changed.

// vectorDir resolves the directory holding the shared payload vectors.
// Resolution only; whether the directory can be read is decided in one place, by
// readVector.
func vectorDir() string {
	return filepath.Join("..", "..", "..", "..", "aggrega-abi", "testdata")
}

// readVector reads one shared vector file and returns its contents. A run that
// could not read the vectors has verified nothing, so this refuses with a clear
// diagnostic naming the path rather than skipping quietly.
func readVector(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(vectorDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("shared payload vector %s is not readable at %s: %v; run scripts/setup-workspace.sh from the umbrella checkout so the sibling ABI checkout is present", name, path, err)
	}
	return b
}

// readHexVector reads one shared canonical-CBOR vector and decodes its hex.
func readHexVector(t *testing.T, name string) []byte {
	t.Helper()
	raw := strings.TrimSpace(string(readVector(t, name)))
	b, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("shared vector %s does not hold decodable hex: %v", name, err)
	}
	return b
}

// TestEncodeParseSourceRequest_MatchesSharedVector proves the request the kit
// builds is byte-identical to the shared ParseInput vector, for the very field
// values that vector's companion document declares. A kit still building a text
// request fails here, because a text document and the canonical binary encoding
// of the same value are different bytes.
func TestEncodeParseSourceRequest_MatchesSharedVector(t *testing.T) {
	t.Parallel()

	// The companion document carries the field values in the textual encoding,
	// where the source bytes are base64. Read them from there rather than
	// restating them, so the two vectors cannot drift apart from this test.
	var golden struct {
		Format  string          `json:"format"`
		Bytes   string          `json:"bytes"`
		Options json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(readVector(t, "ParseInput.golden.json"), &golden); err != nil {
		t.Fatalf("ParseInput.golden.json is not readable as a ParseInput: %v", err)
	}
	source, err := base64.StdEncoding.DecodeString(golden.Bytes)
	if err != nil {
		t.Fatalf("ParseInput.golden.json source bytes are not base64: %v", err)
	}

	got, err := encodeParseSourceRequest(golden.Format, source, golden.Options)
	if err != nil {
		t.Fatalf("encodeParseSourceRequest: %v", err)
	}
	want := readHexVector(t, "ParseInput.cbor.hex")
	if !bytes.Equal(got, want) {
		t.Errorf("request bytes = %x\nwant                 %x", got, want)
	}
}

// TestDecodeParseSourceReply_FailureFromSharedVector proves the kit reads a
// reported failure out of the shared ParseSourceResponse vector and surfaces it
// as a HostError carrying both the machine-matchable code and the message.
func TestDecodeParseSourceReply_FailureFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(readVector(t, "ParseSourceResponse.golden.json"), &golden); err != nil {
		t.Fatalf("ParseSourceResponse.golden.json is not readable as a ParseSourceResponse: %v", err)
	}

	tree, err := decodeParseSourceReply(readHexVector(t, "ParseSourceResponse.cbor.hex"))
	if err == nil {
		t.Fatalf("a failure reply decoded as a success carrying tree %q", tree)
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("decodeParseSourceReply returned %T (%v); want a *HostError", err, err)
	}
	if he.Code != golden.ErrorCode {
		t.Errorf("HostError code = %q; want the vector's %q", he.Code, golden.ErrorCode)
	}
	if he.Message != golden.Error {
		t.Errorf("HostError message = %q; want the vector's %q", he.Message, golden.Error)
	}
}

// TestDecodeParseSourceReply_SuccessShape proves the kit reads the success shape
// too — the one production guests take, and the only one carrying the tree.
//
// Both shapes are asserted because each hides the other's defect: a decoder that
// only reads the failure fields passes any test built from failure replies alone,
// and one that only reads the tree passes any test built from success replies
// alone.
//
// The vector is authored from the canonical encoding's own rules rather than
// captured, and it is short enough to read off:
//
//	a1                     map, 1 pair
//	64 74 72 65 65         text, 4 bytes: "tree"
//	47 7b 22 61 22 3a 31 7d byte run, 7 bytes: {"a":1}
//
// One pair is what a tree-only envelope carries, since the two failure fields are
// omitted when empty; and the tree travels as a byte run holding its own document
// verbatim, which is why an over- or under-declared pair count would move the
// head byte and fail here rather than pass silently.
func TestDecodeParseSourceReply_SuccessShape(t *testing.T) {
	t.Parallel()

	reply, err := hex.DecodeString("a16474726565477b2261223a317d")
	if err != nil {
		t.Fatalf("the authored success vector is not decodable hex: %v", err)
	}
	tree, err := decodeParseSourceReply(reply)
	if err != nil {
		t.Fatalf("decodeParseSourceReply on the success shape: %v", err)
	}
	if string(tree) != `{"a":1}` {
		t.Errorf("tree = %q; want %q", tree, `{"a":1}`)
	}
}

// TestDecodeParseSourceReply_RefusesTheOtherEncoding proves the reply decoder is
// not silently tolerant of a textual reply. Without this, a host left on the
// other encoding would be reported as some downstream shape error rather than as
// an undecodable reply, and the two are not the same diagnosis.
func TestDecodeParseSourceReply_RefusesTheOtherEncoding(t *testing.T) {
	t.Parallel()

	textual := []byte(`{"tree":{"a":1}}`)
	if _, err := decodeParseSourceReply(textual); err == nil {
		t.Fatalf("a textual reply decoded without error; the reply slot carries the binary encoding only")
	}
}

// TestDecodeParseSourceReply_EmptyReplyIsAnError proves an envelope carrying
// neither a tree nor a failure is reported rather than passing as an empty tree.
// The distinction matters because an empty tree is itself a legitimate outcome, so
// a lenient decode would make "the host said nothing" and "the parser produced
// nothing" indistinguishable.
func TestDecodeParseSourceReply_EmptyReplyIsAnError(t *testing.T) {
	t.Parallel()

	// a0: map, 0 pairs — every field omitted.
	if _, err := decodeParseSourceReply([]byte{0xa0}); err == nil {
		t.Fatalf("an empty envelope decoded as a success")
	}
}
