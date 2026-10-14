// Command honk-me is the old name of honk, kept so existing scripts keep working. It is the
// same program (internal/cli): same subcommands, flags, output and exit codes. New installs
// use the honk command:
//
//	go install github.com/honk-me/honk-go/cmd/honk@latest
package main

import "github.com/honk-me/honk-go/internal/cli"

func main() { cli.Main() }
