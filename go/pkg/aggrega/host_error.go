// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package aggrega

import abi "github.com/aggrega-ai/aggrega-abi-go/pkg/abi"

// HostError is the error a host-function wrapper returns when the host reports
// an in-band failure. It carries the machine-matchable Code beside the
// free-form human Message. Error returns the Message, so a consumer that only
// logs or wraps the error is unaffected by the added code; a consumer that
// wants to branch on the failure matches with errors.As and compares Code
// against one of the re-exported Code* constants:
//
//	var he *aggrega.HostError
//	if errors.As(err, &he) && he.Code == aggrega.CodeSourceEscape {
//		// handle a sandbox-escape refusal specifically
//	}
//
// Unknown- and absent-code tolerance is built in: an older host that sends no
// code leaves Code empty, and a newer host that sends a code this build predates
// leaves Code set to that unrecognized value. Either way the comparison against
// a known constant simply does not match and the consumer falls back to the
// Message — there is no panic and no dropped error. Code is therefore a
// best-effort machine axis layered over the always-present message.
type HostError struct {
	Code    string
	Message string
}

func (e *HostError) Error() string { return e.Message }

// The host-function error codes, re-exported from the portable aggrega-abi
// module so a plugin author writes aggrega.Code* while the vocabulary stays
// single-sourced in the schema. The values are the closed set the host sets on
// a host-function reply envelope's error_code field.
const (
	CodeFetchRequestUnreadable     = abi.CodeFetchRequestUnreadable
	CodeFetchInputUnavailable      = abi.CodeFetchInputUnavailable
	CodeFetchFailed                = abi.CodeFetchFailed
	CodeRequestUnreadable          = abi.CodeRequestUnreadable
	CodeRequestUndecodable         = abi.CodeRequestUndecodable
	CodeSourceRootUnavailable      = abi.CodeSourceRootUnavailable
	CodeSourceReadFailed           = abi.CodeSourceReadFailed
	CodeUnknownSelectionOp         = abi.CodeUnknownSelectionOp
	CodeSourceEscape               = abi.CodeSourceEscape
	CodeProducerConsumptionReentry = abi.CodeProducerConsumptionReentry
	CodeNoSourceParser             = abi.CodeNoSourceParser
	CodeNoParserForFormat          = abi.CodeNoParserForFormat
	CodeParseFailed                = abi.CodeParseFailed
)
