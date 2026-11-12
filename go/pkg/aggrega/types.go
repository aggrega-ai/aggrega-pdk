// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package aggrega

// The socket wire types are no longer hand-authored in this PDK. They are
// generated from the canonical schemas into the portable aggrega-abi module and
// re-exported from abi_reexport.go, single-sourcing the wire shape across the
// host, the guest codec, and every PDK. The policy socket's graph is
// the full capability-agnostic IR graph (types in aggrega-abi generated from
// ir-v1beta1), so the IR types are re-exported alongside the policy envelope.
