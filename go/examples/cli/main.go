// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Example cli plugin for the Aggrega Wasm runtime.
//
// It contributes one runtime subcommand — "echo-args" — that echoes the
// argument vector it receives back to standard output, then exits with code
// zero. It is the minimal shape of a CLI plugin: a describe() self-report that
// declares plugin_type "cli", a one-element commands list carrying the
// command's name and short description, and the cli_run export, plus a cli_run
// body that reads its arguments and streams a line of output.
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

// pluginDescriptor is the static describe() self-report: a v1alpha1 cli plugin
// contributing a single command, "echo-args". Each entry in the commands list
// carries a name — the token the host installs the subcommand under — and a
// description, its short help. The cli_run export declares the stdout streaming
// host function it imports, which the host grants under the runtime CLI profile.
const pluginDescriptor = `{"abi_version":"v1alpha1","plugin_type":"cli","commands":[{"name":"echo-args","description":"Echo the received argument vector to standard output."}],"exports":[{"name":"cli_run","host_functions":["aggrega_cli_write_stdout"]}],"metadata":{"author":"aggrega","license":"Apache-2.0"}}`

//go:wasmexport describe
func describe() int32 {
	pdk.Output([]byte(pluginDescriptor))
	return 0
}

//go:wasmexport cli_run
func cli_run() int32 {
	args, err := aggrega.CLIArgs()
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	aggrega.WriteStdout([]byte(strings.Join(args, " ") + "\n"))
	return 0
}

func main() {}
