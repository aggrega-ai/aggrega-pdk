// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import (
	"errors"
	"fmt"
)

// This file carries the guest side of the two source callbacks' wire —
// aggrega_pack_source and aggrega_read_source_file — as request builders and
// reply readers that touch no Wasm memory and make no host call.
//
// They are split out from the bindings so the wire is exercisable where the
// bindings are not: a binding lives behind a wasip1 build constraint and calls
// an import that only a running host resolves, so a test on the host cannot
// reach it. These four can be, and the kit's own test drives them against the
// shared payload vectors.

// encodePackSourceRequest builds the request bytes for one aggrega_pack_source
// call. op names the shape of the selection ("file", "dir", or "tree") and path
// names it relative to the source root.
//
// The request slot on this call carries canonical CBOR, so the bytes are an
// opaque byte run rather than UTF-8 text, and they come from the payload type's
// own generated encoder rather than from anything assembled here.
func encodePackSourceRequest(op, path string) ([]byte, error) {
	b, err := SourcePackRequest{Op: op, Path: path}.EncodeCBOR()
	if err != nil {
		return nil, fmt.Errorf("aggrega_pack_source: encode request: %w", err)
	}
	return b, nil
}

// decodePackSourceReply reads one aggrega_pack_source reply and returns the
// packed source it carries, or the failure it reports.
//
// The reply slot carries canonical CBOR in both the success and the failure case:
// a guest reads one slot and cannot know before decoding which of the two it
// holds, so one decoder reads both. A reported failure surfaces as a HostError
// carrying the host's machine-matchable code beside its message; a reply carrying
// neither a projection nor a failure is itself an error, so an empty reply cannot
// pass as an empty pack.
func decodePackSourceReply(reply []byte) (*SourceProjection, error) {
	var resp SourcePackResponse
	if err := resp.DecodeCBOR(reply); err != nil {
		return nil, fmt.Errorf("aggrega_pack_source: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, &HostError{Code: resp.HostfuncErrorCode, Message: resp.Error}
	}
	if resp.SourceProjection == nil {
		return nil, errors.New("aggrega_pack_source: response carries neither a source projection nor an error")
	}
	return resp.SourceProjection, nil
}

// encodeReadSourceFileRequest builds the request bytes for one
// aggrega_read_source_file call. path names the file relative to the source root.
//
// The request slot carries canonical CBOR, on the same terms as
// encodePackSourceRequest.
func encodeReadSourceFileRequest(path string) ([]byte, error) {
	b, err := SourceFileRequest{Path: path}.EncodeCBOR()
	if err != nil {
		return nil, fmt.Errorf("aggrega_read_source_file: encode request: %w", err)
	}
	return b, nil
}

// decodeReadSourceFileReply reads one aggrega_read_source_file reply and returns
// the file's bytes, or the failure it reports.
//
// Unlike decodePackSourceReply this does not treat an absent payload as an error:
// an empty file is a legitimate read, and its reply carries no content key at all
// because the field is omitted when empty. So a reply with no failure yields an
// empty byte slice, which is the file's actual content.
func decodeReadSourceFileReply(reply []byte) ([]byte, error) {
	var resp SourceFileResponse
	if err := resp.DecodeCBOR(reply); err != nil {
		return nil, fmt.Errorf("aggrega_read_source_file: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, &HostError{Code: resp.HostfuncErrorCode, Message: resp.Error}
	}
	return resp.Content, nil
}
