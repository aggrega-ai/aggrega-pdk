// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/// The Aggrega Wasm plugin ABI version this PDK targets. The constant
/// is the `v1alpha1` contract floor; cross-language PDKs (Go, Rust,
/// TypeScript) export the same value.
pub const API_VERSION: &str = "v1alpha1";

/// Expands to the optional build-provenance fragment a plugin appends
/// inside its `describe()` descriptor's `"metadata"` object.
///
/// The macro reads the `AGGREGA_VERSION` and `AGGREGA_COMMIT`
/// compile-time environment variables and yields a `String`. Each
/// variable that is set contributes a leading-comma JSON member — for
/// example `,"version":"1.2.3","commit":"abc1234"` — so the fragment
/// drops straight in after the fixed metadata members:
///
/// ```
/// use aggrega_pdk_rust::provenance_metadata;
/// let head = r#"{"abi_version":"v1alpha1","metadata":{"author":"me""#;
/// let descriptor = format!("{head}{}}}}}", provenance_metadata!());
/// ```
///
/// When neither variable is set the fragment is empty, keeping an
/// unstamped plugin's descriptor byte-identical to a stamped one's
/// minus the provenance members. Stamp the values at build time:
///
/// ```sh
/// AGGREGA_VERSION=1.2.3 AGGREGA_COMMIT=abc1234 \
///   cargo build --release --target wasm32-wasip1
/// ```
///
/// The macro must expand in the plugin's own crate rather than resolve
/// inside a PDK library function: `option_env!` is evaluated where it is
/// expanded, so a library call would capture the environment at the
/// PDK's own (cached) compile and report a stale value when the author
/// later rebuilds their plugin with the variables set. Expanding in the
/// caller's crate captures the variables at the plugin's compile.
#[macro_export]
macro_rules! provenance_metadata {
    () => {{
        let mut s = ::std::string::String::new();
        if let ::std::option::Option::Some(v) = ::core::option_env!("AGGREGA_VERSION") {
            s.push_str(&::std::format!(r#","version":"{v}""#));
        }
        if let ::std::option::Option::Some(c) = ::core::option_env!("AGGREGA_COMMIT") {
            s.push_str(&::std::format!(r#","commit":"{c}""#));
        }
        s
    }};
}
