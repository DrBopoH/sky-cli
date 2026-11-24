package main

import (
	"os"

	"github.com/DrBopoH/sky-cli/internal/cli"
	"github.com/DrBopoH/sky-cli/internal/ops"
	"github.com/DrBopoH/sky-cli/internal/readme"
)

// main — точка входа: единственное место, где консоль встречается с адаптером патча.
func main() {
	cwd, err := os.Getwd()
	if err != nil {
		os.Stderr.WriteString("✗ " + err.Error() + "\n")
		os.Exit(1)
	}
	var docs []readme.Entry
	for _, o := range ops.All() {
		docs = append(docs, readme.Entry{Usage: o.Name, Summary: o.Doc})
	}
	os.Exit(cli.New(ops.Build, docs).Run(cwd, os.Args[1:], os.Stdout, os.Stderr))
}
