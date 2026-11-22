package main

import (
	"os"

	"github.com/DrBopoH/sky-cli/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		os.Stderr.WriteString("✗ " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(cli.New().Run(cwd, os.Args[1:], os.Stdout, os.Stderr))
}
