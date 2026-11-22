// Package scaffold — шаблоны файлов, которые sky init создаёт в новом проекте.
package scaffold

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed text/gitignore.tmpl
var gitignore string

//go:embed text/readme.tmpl
var readme string

func Gitignore() string { return gitignore }

// Readme собирает README.md: имя проекта и список папок scope.
func Readme(name string, scope []string) string {
	var b strings.Builder
	for _, s := range scope {
		fmt.Fprintf(&b, "- `%s` — исходный код\n", s)
	}
	r := strings.Replace(readme, "{{name}}", name, 1)
	return strings.Replace(r, "{{structure}}", strings.TrimRight(b.String(), "\n"), 1)
}
