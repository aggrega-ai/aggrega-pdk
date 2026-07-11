// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Build-provenance macro tests for the Aggrega Rust PDK.
//!
//! `provenance_metadata!` reads the `AGGREGA_VERSION` / `AGGREGA_COMMIT`
//! compile-time environment variables. Because it is a macro it expands
//! in this test crate, so `option_env!` captures the environment at this
//! crate's compile — proving the value tracks the plugin author's build,
//! not the PDK library's (already-compiled) build. Run with the
//! variables set to see the stamped fragment; run without them to see
//! the empty fragment.

use aggrega_pdk_rust::provenance_metadata;

/// The macro yields exactly the fragment implied by the environment at
/// this crate's compile: empty when neither variable is set, and a
/// leading-comma JSON member per set variable otherwise.
#[test]
fn provenance_reflects_compile_env() {
    let fragment = provenance_metadata!();
    eprintln!("provenance fragment: {fragment:?}");

    match option_env!("AGGREGA_VERSION") {
        Some(v) => assert!(
            fragment.contains(&format!(r#","version":"{v}""#)),
            "expected version member for {v}, got {fragment:?}"
        ),
        None => assert!(
            !fragment.contains("\"version\""),
            "expected no version member, got {fragment:?}"
        ),
    }

    match option_env!("AGGREGA_COMMIT") {
        Some(c) => assert!(
            fragment.contains(&format!(r#","commit":"{c}""#)),
            "expected commit member for {c}, got {fragment:?}"
        ),
        None => assert!(
            !fragment.contains("\"commit\""),
            "expected no commit member, got {fragment:?}"
        ),
    }
}

/// When neither variable is set the fragment is empty, keeping an
/// unstamped descriptor byte-identical to a stamped one minus the
/// provenance members. This assertion is meaningful only on a plain
/// build; when the variables are set the test above covers the stamped
/// shape.
#[test]
fn unstamped_fragment_is_empty() {
    if option_env!("AGGREGA_VERSION").is_none() && option_env!("AGGREGA_COMMIT").is_none() {
        assert_eq!(provenance_metadata!(), "");
    }
}
