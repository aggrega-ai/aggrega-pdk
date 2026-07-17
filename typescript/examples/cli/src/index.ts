// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

/**
 * Example cli plugin for the Aggrega Wasm runtime.
 *
 * It contributes one runtime subcommand — `echo-args` — that echoes the
 * argument vector it receives back to standard output, then exits with code
 * zero. It is the minimal shape of a CLI plugin: a describe() self-report
 * declaring plugin_type `cli`, a one-element commands list carrying the
 * command's name, short description, and one flag (--upper), and the cli_run
 * export, plus a cli_run body that reads its arguments, honors the flag, and
 * streams a line of output. The descriptor is built with typed PDK values and
 * encoded through encodeDescriptor rather than hand-written JSON.
 *
 * Entry convention follows the Extism JS PDK: parameterless functions exposed
 * via module.exports. The plugin reads its argument vector through the PDK's
 * cliArgs() helper and streams output through the PDK's writeStdout() binding.
 */

import {
  API_VERSION,
  cliArgs,
  encodeDescriptor,
  type PluginDescriptor,
  writeStdout,
} from "@aggrega-ai/aggrega-pdk-typescript";

// The long name of the --upper flag echo-args declares. When it appears in the
// argument vector, cli_run echoes the remaining arguments upper-cased.
const UPPER_FLAG = "--upper";

// The static describe() self-report: a v1alpha1 cli plugin contributing a
// single command, "echo-args", carrying one flag. The command's name is the
// token the host installs the subcommand under; its flags are registered on the
// command so `aggrega help echo-args` lists them like a core command's flags.
// The cli_run export declares the stdout streaming host function it imports,
// which the host grants under the runtime CLI profile.
function descriptor(): PluginDescriptor {
  return {
    abi_version: API_VERSION,
    plugin_type: "cli",
    exports: [
      { name: "cli_run", host_functions: ["aggrega_cli_write_stdout"] },
    ],
    commands: [
      {
        name: "echo-args",
        description: "Echo the received argument vector to standard output.",
        flags: [
          {
            name: "upper",
            description: "Echo the arguments upper-cased.",
            type: "bool",
            default: "false",
          },
        ],
      },
    ],
    metadata: { author: "aggrega", license: "Apache-2.0" },
  };
}

// The host calls describe() once at load to read the plugin's static
// self-report before dispatching cli_run.
function describe(): void {
  Host.outputString(encodeDescriptor(descriptor()));
}

function cli_run(): number {
  const args = cliArgs();
  const upper = args.includes(UPPER_FLAG);
  const kept = args.filter((a) => a !== UPPER_FLAG);
  let line = kept.join(" ");
  if (upper) {
    line = line.toUpperCase();
  }
  writeStdout(Memory.fromString(line + "\n").readBytes());
  return 0;
}

module.exports = { describe, cli_run };
