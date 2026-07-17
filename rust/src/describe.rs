// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Typed `describe()` descriptor builder.
//!
//! A guest constructs a typed [`PluginDescriptor`](crate::PluginDescriptor) —
//! plugin type, exports with their host-function and capability fields,
//! capabilities, a provider contract, commands and their flags, metadata — and
//! encodes it to the wire bytes the host reads once at load, rather than
//! hand-assembling the JSON. The codec omits every absent optional field, so a
//! migrated guest emits the same bytes it did as a hand-written JSON literal.

use crate::PluginDescriptor;

/// Encodes a plugin's `describe()` self-report to the wire bytes the host reads
/// once at load. The encoding is the ABI codec's canonical form: fields in
/// schema-declaration order, absent optionals omitted.
///
/// To fold the build-provenance stamp (`AGGREGA_VERSION` / `AGGREGA_COMMIT`)
/// into the descriptor's metadata, use the [`describe!`](crate::describe) macro,
/// which must expand in the plugin's own crate so `option_env!` captures the
/// plugin's compile-time environment rather than the PDK's.
pub fn encode_descriptor(descriptor: &PluginDescriptor) -> String {
    serde_json::to_string(descriptor).expect("a PluginDescriptor is always serializable to JSON")
}

/// Encodes a plugin's `describe()` self-report to its wire bytes, folding the
/// build-provenance stamp into the descriptor's metadata.
///
/// The macro reads the `AGGREGA_VERSION` and `AGGREGA_COMMIT` compile-time
/// environment variables. Each variable that is set populates the corresponding
/// metadata field, allocating the metadata object when the descriptor declared
/// none. An unstamped build (neither variable set) leaves metadata untouched,
/// so the descriptor's wire form is byte-identical to one built without the
/// provenance fields. Stamp the values at build time:
///
/// ```sh
/// AGGREGA_VERSION=1.2.3 AGGREGA_COMMIT=abc1234 \
///   cargo build --release --target wasm32-wasip1
/// ```
///
/// ```
/// use aggrega_pdk_rust::{describe, PluginDescriptor, PluginExport, API_VERSION};
/// let wire = describe!(PluginDescriptor {
///     abi_version: API_VERSION.to_string(),
///     plugin_type: "cli".to_string(),
///     exports: vec![PluginExport { name: "cli_run".to_string(), ..Default::default() }],
///     ..Default::default()
/// });
/// ```
///
/// The macro must expand in the plugin's own crate rather than resolve inside a
/// PDK library function: `option_env!` is evaluated where it is expanded, so a
/// library call would capture the environment at the PDK's own (cached) compile
/// and report a stale value when the author later rebuilds their plugin with the
/// variables set.
#[macro_export]
macro_rules! describe {
    ($descriptor:expr) => {{
        let mut __desc: $crate::PluginDescriptor = $descriptor;
        if let ::core::option::Option::Some(__v) = ::core::option_env!("AGGREGA_VERSION") {
            __desc
                .metadata
                .get_or_insert_with(::core::default::Default::default)
                .version = __v.to_string();
        }
        if let ::core::option::Option::Some(__c) = ::core::option_env!("AGGREGA_COMMIT") {
            __desc
                .metadata
                .get_or_insert_with(::core::default::Default::default)
                .commit = __c.to_string();
        }
        $crate::encode_descriptor(&__desc)
    }};
}
