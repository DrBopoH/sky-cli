package cli

import (
	"github.com/DrBopoH/sky-cli/internal/ops"
	"github.com/DrBopoH/sky-cli/internal/readme"
)

// newApp — сборка консоли для тестов, как в точке входа.
func newApp() *App {
	var docs []readme.Entry
	for _, o := range ops.All() {
		docs = append(docs, readme.Entry{Usage: o.Name, Summary: o.Doc})
	}
	return New(ops.Build, docs)
}
