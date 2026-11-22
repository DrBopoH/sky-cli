// Package scaffold — шаблоны файлов, которые sky init создаёт в новом проекте.
package scaffold

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/conf"
)

//go:embed text/gitignore.tmpl
var gitignore string

//go:embed text/readme.tmpl
var readme string

func Gitignore() string { return gitignore }

// Readme собирает README.md: имя проекта, список папок scope и, если лицензия задана, раздел о ней.
func Readme(name string, scope []string, lic *conf.License) string {
	var b strings.Builder
	for _, s := range scope {
		fmt.Fprintf(&b, "- `%s` — исходный код\n", s)
	}
	r := strings.Replace(readme, "{{name}}", name, 1)
	r = strings.Replace(r, "{{structure}}", strings.TrimRight(b.String(), "\n"), 1)
	switch {
	case lic != nil && lic.Type != "":
		r += "\n## Лицензия\n\n" + lic.Type + ", см. [LICENSE](LICENSE).\n"
	case lic != nil && lic.URL != "":
		r += "\n## Лицензия\n\n[Условия лицензии](" + lic.URL + ").\n"
	}
	return r
}
