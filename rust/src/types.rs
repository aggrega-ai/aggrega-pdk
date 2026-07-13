// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! The socket payload types are no longer hand-authored in this PDK. They are
//! generated from the canonical schemas into the portable `aggrega-abi` crate
//! and re-exported from `lib.rs`, single-sourcing the wire shape across the
//! host, the guest codec, and every PDK. The policy-validator socket's graph is
//! the full provider-agnostic IR graph (generated from ir-v1alpha1), so the IR
//! types are re-exported alongside the policy envelope.
