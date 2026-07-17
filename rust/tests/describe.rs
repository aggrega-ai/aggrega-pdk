// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Descriptor-builder tests for the Aggrega Rust PDK.
//!
//! `encode_descriptor` marshals a typed [`PluginDescriptor`] to the wire bytes
//! the host reads once at load, omitting every absent optional field. The
//! `describe!` macro layers the build-provenance stamp on top: it reads the
//! `AGGREGA_VERSION` / `AGGREGA_COMMIT` compile-time environment variables and
//! folds each set value into the descriptor's metadata. Because it is a macro it
//! expands in this test crate, so `option_env!` captures the environment at this
//! crate's compile — proving the value tracks the plugin author's build, not the
//! PDK library's (already-compiled) build.

use aggrega_pdk_rust::{
    describe, encode_descriptor, ArtifactDelimiters, AssemblyNameAnnotation, InputKinds,
    PluginCLICommand, PluginCommandFlag, PluginDescriptor, PluginDescriptorMeta, PluginExport,
    ProviderContract, API_VERSION,
};
use serde_json::value::RawValue;

/// A CLI descriptor that declares no render-behavior contract marshals WITHOUT a
/// "contract" key (and without any other absent optional), while carrying its
/// command's flags — the wire-faithful omission a migrated guest depends on.
#[test]
fn encode_omits_absent_optionals() {
    let d = PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "cli".to_string(),
        exports: vec![PluginExport {
            name: "cli_run".to_string(),
            host_functions: vec!["aggrega_cli_write_stdout".to_string()],
            ..Default::default()
        }],
        commands: vec![PluginCLICommand {
            name: "echo-args".to_string(),
            description: "Echo the received argument vector to standard output.".to_string(),
            flags: vec![PluginCommandFlag {
                name: "upper".to_string(),
                description: "Echo the arguments upper-cased.".to_string(),
                r#type: "bool".to_string(),
                default: "false".to_string(),
                ..Default::default()
            }],
        }],
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    };
    let wire = encode_descriptor(&d);
    eprintln!("cli descriptor wire: {wire}");
    for key in [
        "\"contract\"",
        "\"stage\"",
        "\"settings_schema\"",
        "\"capabilities\"",
        "\"version\"",
        "\"commit\"",
    ] {
        assert!(!wire.contains(key), "absent optional {key} leaked: {wire}");
    }
    for want in ["\"commands\"", "\"flags\"", "\"upper\"", "\"bool\""] {
        assert!(wire.contains(want), "expected {want} in wire: {wire}");
    }
}

/// A provider descriptor with a fully populated contract marshals the contract
/// and round-trips: decoding the wire and re-encoding yields byte-identical
/// output, and the opaque settings_schema is carried verbatim.
#[test]
fn encode_provider_full_contract_round_trips() {
    let d = PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        description: "Render one IR node and assemble the dispatch group.".to_string(),
        capabilities: vec!["helm".to_string()],
        exports: vec![
            PluginExport {
                name: "render_node".to_string(),
                input_pattern: "b".to_string(),
                stability: "experimental".to_string(),
                ..Default::default()
            },
            PluginExport {
                name: "assemble_boundary".to_string(),
                input_pattern: "b".to_string(),
                ..Default::default()
            },
        ],
        contract: Some(Box::new(ProviderContract {
            input_kinds: InputKinds {
                sourced: true,
                synthesized: true,
                artifacts_standalone: false,
            },
            boundary_assembly: "merge".to_string(),
            artifact_delimiters: Some(Box::new(ArtifactDelimiters {
                left: "[[".to_string(),
                right: "]]".to_string(),
            })),
            source_filter: vec!["templates/".to_string(), "values.yaml".to_string()],
            assembly_name_annotation: Some(Box::new(AssemblyNameAnnotation {
                annotation: "aggrega.ai/release-name".to_string(),
                require_unique: true,
            })),
            reusable_instance: true,
            ..Default::default()
        })),
        settings_schema: Some(
            RawValue::from_string(r#"{"type":"object"}"#.to_string()).expect("valid JSON"),
        ),
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    };
    let wire = encode_descriptor(&d);
    eprintln!("provider descriptor wire: {wire}");
    for want in [
        "\"contract\"",
        "\"boundary_assembly\":\"merge\"",
        "\"artifact_delimiters\"",
        "\"[[\"",
        "\"source_filter\"",
        "\"assembly_name_annotation\"",
        "\"require_unique\":true",
        "\"reusable_instance\":true",
        "\"settings_schema\":{\"type\":\"object\"}",
    ] {
        assert!(wire.contains(want), "expected {want} in wire: {wire}");
    }

    // Round-trip: decode the wire and re-encode; the bytes are stable.
    let decoded: PluginDescriptor = serde_json::from_str(&wire).expect("decode descriptor");
    let reencoded = encode_descriptor(&decoded);
    assert_eq!(wire, reencoded, "re-encode not byte-stable");
}

/// encode_descriptor carries explicit metadata version/commit fields (the typed
/// metadata-append path), and omits them when unset.
#[test]
fn encode_metadata_version_commit_fields() {
    let stamped = PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        exports: vec![PluginExport {
            name: "render_node".to_string(),
            input_pattern: "b".to_string(),
            ..Default::default()
        }],
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            version: "1.2.3".to_string(),
            commit: "abc1234".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    };
    let wire = encode_descriptor(&stamped);
    for want in ["\"version\":\"1.2.3\"", "\"commit\":\"abc1234\""] {
        assert!(wire.contains(want), "expected {want} in wire: {wire}");
    }

    let bare = PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "provider".to_string(),
        exports: vec![PluginExport {
            name: "render_node".to_string(),
            ..Default::default()
        }],
        ..Default::default()
    };
    let wire = encode_descriptor(&bare);
    for key in ["\"version\"", "\"commit\"", "\"metadata\""] {
        assert!(!wire.contains(key), "unstamped bare descriptor leaked {key}: {wire}");
    }
}

/// The describe! macro folds the build-provenance stamp from the compile-time
/// environment into the descriptor's metadata, allocating the metadata object
/// when the descriptor declared none. The assertion is env-aware: it verifies
/// the stamped member for a set variable and its absence otherwise.
#[test]
fn describe_macro_reflects_compile_env() {
    let base = || PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "cli".to_string(),
        exports: vec![PluginExport {
            name: "cli_run".to_string(),
            ..Default::default()
        }],
        ..Default::default()
    };

    let wire = describe!(base());
    eprintln!("describe! wire: {wire}");

    match option_env!("AGGREGA_VERSION") {
        Some(v) => assert!(
            wire.contains(&format!("\"version\":\"{v}\"")),
            "expected version member for {v}, got {wire}"
        ),
        None => assert!(
            !wire.contains("\"version\""),
            "expected no version member, got {wire}"
        ),
    }
    match option_env!("AGGREGA_COMMIT") {
        Some(c) => assert!(
            wire.contains(&format!("\"commit\":\"{c}\"")),
            "expected commit member for {c}, got {wire}"
        ),
        None => assert!(
            !wire.contains("\"commit\""),
            "expected no commit member, got {wire}"
        ),
    }

    // The wire is always a valid descriptor regardless of the compile env.
    let decoded: PluginDescriptor = serde_json::from_str(&wire).expect("decode describe! wire");
    assert_eq!(decoded.plugin_type, "cli");
}
