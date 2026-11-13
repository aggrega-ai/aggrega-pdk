// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package aggrega

import (
	"errors"
	"fmt"
	"testing"

	abi "github.com/aggrega-ai/aggrega-abi/go/pkg/abi"
)

// TestHostError_MatchByCode proves a guest branches on a host failure by code
// via errors.As + a Code compare, even when the HostError is wrapped.
func TestHostError_MatchByCode(t *testing.T) {
	err := fmt.Errorf("render node payments: %w", &HostError{Code: CodeSourceEscape, Message: "selection \"../etc\" escapes the source root"})

	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("errors.As did not match a wrapped *HostError")
	}
	if he.Code != CodeSourceEscape {
		t.Errorf("Code = %q; want %q", he.Code, CodeSourceEscape)
	}
}

// TestHostError_ErrorReturnsMessage proves a message-only consumer is
// unaffected by the added code: Error() returns the human message verbatim.
func TestHostError_ErrorReturnsMessage(t *testing.T) {
	const msg = "no parser plugin advertises the requested format"
	he := &HostError{Code: CodeNoParserForFormat, Message: msg}
	if he.Error() != msg {
		t.Errorf("Error() = %q; want the message %q", he.Error(), msg)
	}
}

// TestHostError_AbsentAndUnknownCodeTolerated proves the tolerance contract: an
// absent code (older host) and an unrecognized code (newer host) both leave the
// error usable — a compare against a known constant simply does not match and
// the consumer falls back to the message, with no panic and no dropped error.
func TestHostError_AbsentAndUnknownCodeTolerated(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{"absent (older host)", ""},
		{"unrecognized (newer host)", "a_future_code_this_build_predates"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const msg = "the host reported a failure"
			var err error = &HostError{Code: c.code, Message: msg}

			var he *HostError
			if !errors.As(err, &he) {
				t.Fatalf("errors.As did not match *HostError")
			}
			// A code the guest knows does not match, so the guest falls back.
			matched := he.Code == CodeSourceEscape ||
				he.Code == CodeParseFailed ||
				he.Code == CodeNoParserForFormat
			if matched {
				t.Errorf("code %q unexpectedly matched a known constant", he.Code)
			}
			if he.Error() != msg {
				t.Errorf("Error() = %q; want the message %q preserved", he.Error(), msg)
			}
		})
	}
}

// TestHostError_CodesAreSingleSourced proves the PDK re-exports are the same
// values the abi module (and thus the host) uses — the vocabulary is single-
// sourced in the schema, not a hand-maintained second copy.
func TestHostError_CodesAreSingleSourced(t *testing.T) {
	pairs := map[string]string{
		CodeFetchRequestUnreadable:     abi.CodeFetchRequestUnreadable,
		CodeFetchInputUnavailable:      abi.CodeFetchInputUnavailable,
		CodeFetchFailed:                abi.CodeFetchFailed,
		CodeRequestUnreadable:          abi.CodeRequestUnreadable,
		CodeRequestUndecodable:         abi.CodeRequestUndecodable,
		CodeSourceRootUnavailable:      abi.CodeSourceRootUnavailable,
		CodeSourceReadFailed:           abi.CodeSourceReadFailed,
		CodeUnknownSelectionOp:         abi.CodeUnknownSelectionOp,
		CodeSourceEscape:               abi.CodeSourceEscape,
		CodeProducerConsumptionReentry: abi.CodeProducerConsumptionReentry,
		CodeNoSourceParser:             abi.CodeNoSourceParser,
		CodeNoParserForFormat:          abi.CodeNoParserForFormat,
		CodeParseFailed:                abi.CodeParseFailed,
	}
	for pdkVal, abiVal := range pairs {
		if pdkVal != abiVal {
			t.Errorf("re-export drift: %q != %q", pdkVal, abiVal)
		}
	}
}
