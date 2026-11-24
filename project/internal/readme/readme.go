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

//go:embed text/MT-unify-systems.md
var unifyText string

//go:embed text/MT-fix-before-reset.md
var fixText string

const (
	commandsMarker   = "{{commands}}"
	operationsMarker = "{{operations}}"
	confMarker       = "{{conf}}"
)

type Entry struct {
	Usage   string
	Summary string
}

type File struct {
	Name    string
	Content []byte
}

func bullets(es []Entry) string {
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "- `%s` — %s\n", e.Usage, e.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Files — полный набор файлов .sky/readme/. Usage операции — её имя, Summary — Doc.
func Files(cmds, operations []Entry, confExample string) []File {
	r := strings.Replace(readmeText, commandsMarker, bullets(cmds), 1)
	r = strings.Replace(r, operationsMarker, bullets(operations), 1)
	r = strings.Replace(r, confMarker, strings.TrimRight(confExample, "\n"), 1)
	return []File{
		{Name: "readme.md", Content: []byte(r)},
		{Name: "git-versioning.md", Content: []byte(versioningText)},
		{Name: "MT-unify-systems.md", Content: []byte(unifyText)},
		{Name: "MT-fix-before-reset.md", Content: []byte(fixText)},
	}
}
