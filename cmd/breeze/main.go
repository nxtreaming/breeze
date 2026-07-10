// Command breeze is the Breeze framework's project scaffolding and code
// generation CLI, in the spirit of `rails new` / `rails generate`.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "new":
		err = runNew(args)
	case "generate", "g":
		err = runGenerate(args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "breeze: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "breeze: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `breeze — scaffolding and code generation for the Breeze web framework

Usage:
  breeze new <name> [--template=api|views] [--module=<import-path>]
  breeze generate handler <Name> [--methods=GET,POST,PUT,DELETE] [--force]
  breeze generate resource <Name> field:type [field:type ...] [--plural=<name>] [--force]
  breeze help

Aliases:
  g    generate

Examples:
  breeze new myapp
  breeze new myapp --template=views
  breeze generate handler Session --methods=GET,POST
  breeze generate resource User name:string email:string age:int
`)
}
