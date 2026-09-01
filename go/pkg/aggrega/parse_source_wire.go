// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import (
	"encoding/json"
	"errors"
	"fmt"
)

// This file carries the guest side of the aggrega_parse_source call's wire —
// building the request bytes and reading the reply bytes — as two functions that
// touch no Wasm memory and make no host call.
//
// They are split out from the binding so the wire is exercisable where the
// binding is not: the binding lives behind a wasip1 build constraint and calls
// an import that only a running host resolves, so a test on the host cannot
// reach it. These two can be, and the kit's own test drives them against the
// shared payload vectors.

// encodeParseSourceRequest builds the request bytes for one aggrega_parse_source
// call. format is the syntactic format of the source, source is the raw source
// bytes, and options is an optional opaque options document the parser
// interprets, or nil.
//
// The request slot on this call carries canonical CBOR, so the bytes are an
// opaque byte run rather than UTF-8 text, and they come from the payload type's
// own generated encoder rather than from anything assembled here.
func encodeParseSourceRequest(format string, source []byte, options json.RawMessage) ([]byte, error) {
	b, err := ParseInput{Format: format, Bytes: source, Options: options}.EncodeCBOR()
	if err != nil {
		return nil, fmt.Errorf("aggrega_parse_source: encode request: %w", err)
	}
	return b, nil
}

// decodeParseSourceReply reads one aggrega_parse_source reply and returns the
// canonical tree it carries, or the failure it reports.
//
// The reply slot carries canonical CBOR in both the success and the failure case:
// a guest reads one slot and cannot know before decoding which of the two it
// holds, so one decoder reads both. A reported failure surfaces as a HostError
// carrying the host's machine-matchable code beside its message; a reply carrying
// neither a tree nor a failure is itself an error, so an empty reply cannot pass
// as an empty tree.
func decodeParseSourceReply(reply []byte) (json.RawMessage, error) {
	var resp ParseSourceResponse
	if err := resp.DecodeCBOR(reply); err != nil {
		return nil, fmt.Errorf("aggrega_parse_source: decode reply: %w", err)
	}
	if resp.Error != "" {
		return nil, &HostError{Code: resp.HostfuncErrorCode, Message: resp.Error}
	}
	if len(resp.Tree) == 0 {
		return nil, errors.New("aggrega_parse_source: reply carries neither a tree nor an error")
	}
	return resp.Tree, nil
}
