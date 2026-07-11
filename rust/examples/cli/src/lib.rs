// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Example cli plugin for the Aggrega Wasm runtime.
//!
//! It contributes one runtime subcommand — `echo-args` — that echoes the
//! argument vector it receives back to standard output, then exits with code
//! zero. It is the minimal shape of a CLI plugin: a `describe()` self-report
//! declaring plugin_type `cli`, a command name, a short description, and the
//! `cli_run` export, plus a `cli_run` body that reads its arguments and streams
//! a line of output.
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

use aggrega_pdk_rust::{cli_args, write_stdout};
use extism_pdk::{plugin_fn, FnResult};

/// The static `describe()` self-report: a v1alpha1 `cli` plugin contributing
/// the `echo-args` command. The command token is the name the host installs the
/// subcommand under; the description is its short help. The `cli_run` export
/// declares the stdout streaming host function it imports, which the host grants
/// under the runtime CLI profile.
const PLUGIN_DESCRIPTOR: &str = r#"{"abi_version":"v1alpha1","plugin_type":"cli","command":"echo-args","description":"Echo the received argument vector to standard output.","exports":[{"name":"cli_run","host_functions":["aggrega_cli_write_stdout"]}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}"#;

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `cli_run`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(PLUGIN_DESCRIPTOR.to_string())
}

/// `cli_run` reads the invocation's argument vector, echoes it to standard
/// output as a single line, and returns exit code zero.
#[plugin_fn]
pub fn cli_run() -> FnResult<()> {
    let args = cli_args()?;
    write_stdout(format!("{}\n", args.join(" ")).as_bytes());
    Ok(())
}
