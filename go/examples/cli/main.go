// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Example cli plugin for the Aggrega Wasm runtime.
//
// It contributes one runtime subcommand — "echo-args" — that echoes the
// argument vector it receives back to standard output, then exits with code
// zero. It is the minimal shape of a CLI plugin: a describe() self-report that
// declares plugin_type "cli", a one-element commands list carrying the
// command's name, short description, and one flag (--upper), and the cli_run
// export, plus a cli_run body that reads its arguments, honors the flag, and
// streams a line of output. The descriptor is built with typed PDK values and
// marshaled through aggrega.MarshalDescriptor rather than hand-written JSON.
//
// The host verifies end-to-end that:
//
//   - The plugin loaded through the runtime CLI instantiation profile.
//   - describe() reported a valid cli descriptor with the cli_run export.
//   - cli_run read the CLIRunInput argument vector.
//   - cli_run streamed output through the stdout streaming host function.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o cli-go.wasm ./
package main

import (
	"strings"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// upperFlag is the long name of the --upper flag echo-args declares. When it
// appears in the argument vector, cli_run echoes the remaining arguments
// upper-cased.
const upperFlag = "--upper"

// descriptor is the static describe() self-report: a cli plugin
// contributing a single command, "echo-args", carrying one flag. The command's
// name is the token the host installs the subcommand under; its flags are
// registered on the command so aggrega help echo-args lists them like a core
// command's flags. The cli_run export declares the stdout streaming host
// function it imports, which the host grants under the runtime CLI profile.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion: aggrega.APIVersion,
		PluginType: "cli",
		Exports: []aggrega.PluginExport{
			{Name: "cli_run", HostFunctions: []string{"aggrega_cli_write_stdout"}},
		},
		Commands: []aggrega.PluginCLICommand{{
			Name:        "echo-args",
			Description: "Echo the received argument vector to standard output.",
			Flags: []aggrega.PluginCommandFlag{{
				Name:        "upper",
				Description: "Echo the arguments upper-cased.",
				Type:        "bool",
				Default:     "false",
			}},
		}},
		Metadata: &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

//go:wasmexport describe
func describe() int32 {
	out, err := aggrega.MarshalDescriptor(descriptor())
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

//go:wasmexport cli_run
func cli_run() int32 {
	args, err := aggrega.CLIArgs()
	if err != nil {
		pdk.SetError(err)
		return 1
	}

	upper := false
	kept := args[:0:0]
	for _, arg := range args {
		if arg == upperFlag {
			upper = true
			continue
		}
		kept = append(kept, arg)
	}

	line := strings.Join(kept, " ")
	if upper {
		line = strings.ToUpper(line)
	}
	aggrega.WriteStdout([]byte(line + "\n"))
	return 0
}

func main() {}
