// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Example cli plugin for the Aggrega Wasm runtime.
//!
//! It contributes one runtime subcommand — `echo-args` — that echoes the
//! argument vector it receives back to standard output, then exits with code
//! zero. It is the minimal shape of a CLI plugin: a `describe()` self-report
//! declaring plugin_type `cli`, a one-element commands list carrying the
//! command's name, short description, and one flag (--upper), and the `cli_run`
//! export, plus a `cli_run` body that reads its arguments, honors the flag, and
//! streams a line of output. The descriptor is built with typed PDK values and
//! encoded through `aggrega_pdk_rust::encode_descriptor` rather than hand-written
//! JSON.
//!
//! The host verifies end-to-end that:
//!
//!  - The plugin loaded through the runtime CLI instantiation profile.
//!  - `describe()` reported a valid `cli` descriptor with the `cli_run` export.
//!  - `cli_run` read the [`CLIRunInput`](aggrega_pdk_rust::CLIRunInput) argument
//!    vector.
//!  - `cli_run` streamed output through the stdout streaming host function.
//!
//! Build: `cargo build --release --target wasm32-wasip1`

use aggrega_pdk_rust::{
    cli_args, encode_descriptor, write_stdout, PluginCLICommand, PluginCommandFlag,
    PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};

/// The long name of the --upper flag echo-args declares. When it appears in the
/// argument vector, cli_run echoes the remaining arguments upper-cased.
const UPPER_FLAG: &str = "--upper";

/// The static `describe()` self-report: a v1alpha1 `cli` plugin contributing a
/// single command, `echo-args`, carrying one flag. The command's name is the
/// token the host installs the subcommand under; its flags are registered on the
/// command so `aggrega help echo-args` lists them like a core command's flags.
/// The `cli_run` export declares the stdout streaming host function it imports,
/// which the host grants under the runtime CLI profile.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
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
    }
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `cli_run`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// `cli_run` reads the invocation's argument vector, honors the --upper flag,
/// echoes it to standard output as a single line, and returns exit code zero.
#[plugin_fn]
pub fn cli_run() -> FnResult<()> {
    let args = cli_args()?;
    let upper = args.iter().any(|a| a == UPPER_FLAG);
    let kept: Vec<String> = args.into_iter().filter(|a| a != UPPER_FLAG).collect();
    let mut line = kept.join(" ");
    if upper {
        line = line.to_uppercase();
    }
    write_stdout(format!("{line}\n").as_bytes());
    Ok(())
}
