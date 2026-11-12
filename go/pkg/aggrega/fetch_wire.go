// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package aggrega

import "fmt"

// This file carries the guest side of the aggrega_fetch_external_input reply as
// a reader that touches no Wasm memory and makes no host call, split out from
// the binding for the reason source_wire.go gives: the binding lives behind a
// wasip1 build constraint and calls an import only a running host resolves, so
// a test on the host can reach this reader and cannot reach the call.
//
// The request on this call is a bare URL rather than a payload document, so it
// needs no builder: the binding allocates it as UTF-8 text.

// decodeFetchExternalInputReply reads one aggrega_fetch_external_input reply
// and returns the fetched bytes, or the failure it reports.
//
// The reply slot carries canonical CBOR in both the success and the failure
// case, so one decoder reads both. A reply carrying an error message or an
// error code is a failure and surfaces as a HostError carrying the code beside
// the message. Any other reply is a success, and its content is the payload:
// the content field is omitted when empty, so a zero-length payload arrives as
// a reply with no field at all and yields an empty byte slice rather than an
// error. Reading the failure from the failure fields, rather than from the
// content key's absence, is what keeps that empty payload distinguishable from
// a failure.
func decodeFetchExternalInputReply(reply []byte) ([]byte, error) {
	var resp FetchExternalInputResponse
	if err := resp.DecodeCBOR(reply); err != nil {
		return nil, fmt.Errorf("aggrega_fetch_external_input: decode response: %w", err)
	}
	if resp.Error != "" || resp.HostfuncErrorCode != "" {
		msg := resp.Error
		if msg == "" {
			msg = "aggrega_fetch_external_input: the host reported failure " + resp.HostfuncErrorCode
		}
		return nil, &HostError{Code: resp.HostfuncErrorCode, Message: msg}
	}
	if resp.Content == nil {
		return []byte{}, nil
	}
	return resp.Content, nil
}
