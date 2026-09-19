// Command todo is the todo-cli entry point.
package main

import (
	"os"

	"example.com/todo-cli/src/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
