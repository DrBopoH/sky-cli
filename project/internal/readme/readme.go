// Package readme собирает неизменяемые файлы .sky/readme/: тексты лежат в text/, список команд и пример конфига подставляются.
package readme

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed text/readme.md
var readmeText string

//go:embed text/git-versioning.md
var versioningText string

const (
	commandsMarker = "{{commands}}"
	confMarker     = "{{conf}}"
)

type Entry struct {
	Usage   string
	Summary string
}

type File struct {
	Name    string
	Content []byte
}

// Files — полный набор файлов .sky/readme/.
func Files(cmds []Entry, confExample string) []File {
	var list strings.Builder
	for _, c := range cmds {
		fmt.Fprintf(&list, "- `%s` — %s\n", c.Usage, c.Summary)
	}
	r := strings.Replace(readmeText, commandsMarker, strings.TrimRight(list.String(), "\n"), 1)
	r = strings.Replace(r, confMarker, strings.TrimRight(confExample, "\n"), 1)
	return []File{
		{Name: "readme.md", Content: []byte(r)},
		{Name: "git-versioning.md", Content: []byte(versioningText)},
	}
}
