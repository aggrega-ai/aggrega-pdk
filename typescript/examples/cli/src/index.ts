// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Example cli plugin for the Aggrega Wasm runtime.
 *
 * It contributes one runtime subcommand — `echo-args` — that echoes the
 * argument vector it receives back to standard output, then exits with code
 * zero. It is the minimal shape of a CLI plugin: a describe() self-report
 * declaring plugin_type `cli`, a command name, a short description, and the
 * cli_run export, plus a cli_run body that reads its arguments and streams a
 * line of output.
 *
 * Entry convention follows the Extism JS PDK: parameterless functions exposed
 * via module.exports. The plugin reads its argument vector through the PDK's
 * cliArgs() helper and streams output through the PDK's writeStdout() binding.
 */

import { cliArgs, writeStdout } from "@aggrega-ai/aggrega-pdk-typescript";

// The static describe() self-report: a v1alpha1 cli plugin contributing the
// "echo-args" command. The command token is the name the host installs the
// subcommand under; the description is its short help. The cli_run export
// declares the stdout streaming host function it imports, which the host grants
// under the runtime CLI profile.
const PLUGIN_DESCRIPTOR =
  '{"abi_version":"v1alpha1","plugin_type":"cli","command":"echo-args","description":"Echo the received argument vector to standard output.","exports":[{"name":"cli_run","host_functions":["aggrega_cli_write_stdout"]}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}';

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching cli_run.
function describe(): void {
  Host.outputString(PLUGIN_DESCRIPTOR);
}

function cli_run(): number {
  const args = cliArgs();
  writeStdout(Memory.fromString(args.join(" ") + "\n").readBytes());
  return 0;
}

module.exports = { describe, cli_run };
