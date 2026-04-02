package main

import (
	"fmt"
	"os"

	"github.com/mjmorales/claude-env/cmd/claude-env/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
