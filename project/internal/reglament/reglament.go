// Package reglament собирает записку .sky/clouds/sky-reglament.md; текст лежит в text/, список команд берётся из реестра.
package reglament

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed text/intro.md
var intro string

//go:embed text/versioning.md
var versioning string

type Entry struct {
	Usage   string
	Summary string
}

func Render(version string, cmds []Entry, confExample string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# sky-reglament.md — записка для Claude\n\n")
	fmt.Fprintf(&b, "Сгенерировано sky %s. Файл только для чтения: правка или удаление откатываются при следующем запуске sky. Менять текст можно только в репозитории sky-cli (`project/internal/reglament`).\n\n", version)
	b.WriteString(strings.TrimSpace(intro) + "\n\n")
	b.WriteString(strings.TrimSpace(versioning) + "\n\n")
	b.WriteString("## Команды\n\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "- `%s` — %s\n", c.Usage, c.Summary)
	}
	b.WriteString("\n## Конфиг\n\n`.sky/sky.conf` по умолчанию:\n\n```\n")
	b.WriteString(strings.TrimRight(confExample, "\n"))
	b.WriteString("\n```\n")
	return []byte(b.String())
}
