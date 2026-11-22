// Package license — встроенные тексты открытых лицензий и подстановка полей.
package license

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed text/*.txt
var texts embed.FS

// Fields — значения для подстановки в текст лицензии.
type Fields struct {
	Year   string
	Holder string
	Email  string
}

type spec struct {
	NeedsHolder bool // в тексте есть имя правообладателя
}

var known = map[string]spec{
	"MIT":          {NeedsHolder: true},
	"BSD-3-Clause": {NeedsHolder: true},
	"ISC":          {NeedsHolder: true},
	"Unlicense":    {},
}

// Known — поддерживаемые типы по алфавиту.
func Known() []string {
	out := make([]string, 0, len(known))
	for k := range known {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Check проверяет, что тип известен и все нужные ему поля заданы.
func Check(id string, f Fields) error {
	s, ok := known[id]
	if !ok {
		return fmt.Errorf("неизвестный тип лицензии «%s» (есть: %s)", id, strings.Join(Known(), ", "))
	}
	if s.NeedsHolder && strings.TrimSpace(f.Holder) == "" {
		return fmt.Errorf("для %s нужно поле holder", id)
	}
	return nil
}

// Render возвращает текст лицензии с подставленными полями.
func Render(id string, f Fields) (string, error) {
	if err := Check(id, f); err != nil {
		return "", err
	}
	b, err := texts.ReadFile("text/" + id + ".txt")
	if err != nil {
		return "", err
	}
	holder := strings.TrimSpace(f.Holder)
	if e := strings.TrimSpace(f.Email); e != "" && holder != "" {
		holder += " <" + e + ">"
	}
	r := strings.NewReplacer("{{year}}", f.Year, "{{holder}}", holder)
	return r.Replace(string(b)), nil
}
