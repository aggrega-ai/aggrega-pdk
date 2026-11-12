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

// Wire tests for the aggrega_fetch_external_input reply, driven against the
// shared payload vectors the ABI package publishes rather than against bytes
// produced here, for the reason source_wire_test.go gives: a reader checked only
// against its own encoder round-trips cleanly however wrong both halves are.
//
// The vector readers live in parse_source_wire_test.go, in the same package.

// TestDecodeFetchExternalInputReply_ContentFromSharedVector proves the kit reads
// the fetched bytes out of the shared success vector, whose companion document
// carries them base64 in the textual encoding.
func TestDecodeFetchExternalInputReply_ContentFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(readVector(t, "FetchExternalInputResponseContent.golden.json"), &golden); err != nil {
		t.Fatalf("FetchExternalInputResponseContent.golden.json is not readable as a FetchExternalInputResponse: %v", err)
	}
	want, err := base64.StdEncoding.DecodeString(golden.Content)
	if err != nil {
		t.Fatalf("FetchExternalInputResponseContent.golden.json content is not base64: %v", err)
	}

	got, err := decodeFetchExternalInputReply(readHexVector(t, "FetchExternalInputResponseContent.cbor.hex"))
	if err != nil {
		t.Fatalf("decodeFetchExternalInputReply on the success shape: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("content = %q; want the vector's %q", got, want)
	}
}

// TestDecodeFetchExternalInputReply_EmptyContentFromSharedVector proves a
// zero-length payload is a success yielding empty bytes. Its reply carries no
// field at all, because the content field is omitted when empty, so a reader
// that took an absent content key for a failure fails here.
func TestDecodeFetchExternalInputReply_EmptyContentFromSharedVector(t *testing.T) {
	t.Parallel()

	got, err := decodeFetchExternalInputReply(readHexVector(t, "FetchExternalInputResponseEmptyContent.cbor.hex"))
	if err != nil {
		t.Fatalf("a zero-length payload must read as empty bytes, not as an error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("content = %#v; want an empty, non-nil byte slice", got)
	}
}

// TestDecodeFetchExternalInputReply_FailureFromSharedVector proves the kit reads
// a reported failure out of the shared failure vector and surfaces it as a
// HostError carrying both the machine-matchable code and the message.
func TestDecodeFetchExternalInputReply_FailureFromSharedVector(t *testing.T) {
	t.Parallel()

	var golden struct {
		Error             string `json:"error"`
		HostfuncErrorCode string `json:"hostfunc_error_code"`
	}
	if err := json.Unmarshal(readVector(t, "FetchExternalInputResponse.golden.json"), &golden); err != nil {
		t.Fatalf("FetchExternalInputResponse.golden.json is not readable as a FetchExternalInputResponse: %v", err)
	}

	content, err := decodeFetchExternalInputReply(readHexVector(t, "FetchExternalInputResponse.cbor.hex"))
	if err == nil {
		t.Fatalf("a failure reply decoded as a success carrying content %q", content)
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("decodeFetchExternalInputReply returned %T (%v); want a *HostError", err, err)
	}
	if he.Code != golden.HostfuncErrorCode {
		t.Errorf("HostError code = %q; want the vector's %q", he.Code, golden.HostfuncErrorCode)
	}
	if he.Message != golden.Error {
		t.Errorf("HostError message = %q; want the vector's %q", he.Message, golden.Error)
	}
}

// TestDecodeFetchExternalInputReply_CodeAloneIsAFailure proves a reply carrying
// a failure code and no message is still a failure. Without it, such a reply
// would carry no content and no message, which is exactly the shape of the
// zero-length payload, and would read as a successful empty fetch.
func TestDecodeFetchExternalInputReply_CodeAloneIsAFailure(t *testing.T) {
	t.Parallel()

	reply, err := FetchExternalInputResponse{HostfuncErrorCode: CodeFetchFailed}.EncodeCBOR()
	if err != nil {
		t.Fatalf("encode the code-only reply: %v", err)
	}
	_, err = decodeFetchExternalInputReply(reply)
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("a code-only reply returned %v; want a *HostError", err)
	}
	if he.Code != CodeFetchFailed || he.Message == "" {
		t.Errorf("HostError = %+v; want code %q and a non-empty message", he, CodeFetchFailed)
	}
}

// TestDecodeFetchExternalInputReply_RefusesTheOtherEncoding proves the reader is
// not silently tolerant of a textual reply. Without this, a host left on the
// textual encoding would surface as some downstream shape error rather than as
// an undecodable reply, and the two are not the same diagnosis.
func TestDecodeFetchExternalInputReply_RefusesTheOtherEncoding(t *testing.T) {
	t.Parallel()

	if _, err := decodeFetchExternalInputReply([]byte(`{"content":"aGVsbG8="}`)); err == nil {
		t.Errorf("a textual fetch reply decoded without error; the reply slot carries the binary encoding only")
	}
}

// TestFetchVectorHeads reads the map head byte of each shared fetch vector from
// the canonical encoding's own rules rather than from the encoder that wrote
// them, and the content vector's value head too: a byte string (major type 2),
// which a text-string or base64 carry would not produce.
func TestFetchVectorHeads(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		vector string
		want   byte
		why    string
	}{
		{"FetchExternalInputResponseContent.cbor.hex", 0xa1, "the content alone; both failure fields omitted"},
		{"FetchExternalInputResponseEmptyContent.cbor.hex", 0xa0, "no field at all: an empty content is omitted"},
		{"FetchExternalInputResponse.cbor.hex", 0xa2, "message and code; the content omitted"},
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

	// a1, 67 "content", then the value head: 0x40 | length marks a byte string.
	b := readHexVector(t, "FetchExternalInputResponseContent.cbor.hex")
	if len(b) < 10 || b[9]&0xe0 != 0x40 {
		t.Errorf("FetchExternalInputResponseContent content value head = %x; want a byte string (major type 2)", b)
	}
}
