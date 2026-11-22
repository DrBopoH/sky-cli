// Package conf — формат .sky/sky.conf: строки «ключ = значение», списки в [a, b], комментарии с «#».
package conf

import (
	"fmt"
	"path"
	"strings"
)

const (
	DefaultScope = "project/"
	DefaultInbox = "~/Downloads"
)

type Conf struct {
	Scope      []string // папки проекта от корня репозитория
	Inbox      string   // откуда sky забирает patch*.zip
	Regulation string   // необязательная ссылка на регламент; sky её не загружает
}

func Default() Conf {
	return Conf{Scope: []string{DefaultScope}, Inbox: DefaultInbox}
}

const header = `# sky.conf — пути проекта.
# scope — папки проекта от корня репозитория; sky применяет патчи, снимает «до» и коммитит только в них.
#   бинарь, одна папка:  scope = [project/]
#   веб:                 scope = [backend/, frontend/]
# inbox — папка, куда приходят patch*.zip.
# regulation — необязательная ссылка (URL) на регламент версий; sky её только хранит и не загружает.
`

func Render(c Conf) string {
	reg := "# regulation = https://"
	if c.Regulation != "" {
		reg = "regulation = " + c.Regulation
	}
	return fmt.Sprintf("%s\nscope = [%s]\ninbox = %s\n%s\n", header, strings.Join(c.Scope, ", "), c.Inbox, reg)
}

// Parse читает sky.conf; пропущенные ключи берутся по умолчанию, неизвестные — ошибка.
func Parse(src string) (Conf, error) {
	c := Default()
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return c, fmt.Errorf("sky.conf:%d: нужен формат «ключ = значение»", i+1)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "scope":
			s, err := ParseScope([]string{val})
			if err != nil {
				return c, fmt.Errorf("sky.conf:%d: %w", i+1, err)
			}
			c.Scope = s
		case "inbox":
			if val == "" {
				return c, fmt.Errorf("sky.conf:%d: inbox пуст", i+1)
			}
			c.Inbox = val
		case "regulation":
			if !strings.HasPrefix(val, "https://") && !strings.HasPrefix(val, "http://") {
				return c, fmt.Errorf("sky.conf:%d: regulation — ссылка вида https://…", i+1)
			}
			c.Regulation = val
		default:
			return c, fmt.Errorf("sky.conf:%d: неизвестный ключ «%s»", i+1, key)
		}
	}
	return c, nil
}

// ParseScope принимает «a/ b/», «a/,b/» и «[a/, b/]» (в любой комбинации) и возвращает нормализованный список.
func ParseScope(args []string) ([]string, error) {
	s := strings.TrimSpace(strings.Join(args, " "))
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		p, err := normPath(f)
		if err != nil {
			return nil, err
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("scope пуст")
	}
	return out, nil
}

func normPath(raw string) (string, error) {
	p := strings.Trim(raw, `"'`)
	if p == "" {
		return "", fmt.Errorf("пустой путь в scope")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return "", fmt.Errorf("«%s»: путь должен быть от корня репозитория", raw)
	}
	p = path.Clean(p)
	switch {
	case p == ".":
		return "./", nil
	case p == ".." || strings.HasPrefix(p, "../"):
		return "", fmt.Errorf("«%s»: путь выходит за репозиторий", raw)
	case p == ".git" || p == ".sky" || strings.HasPrefix(p, ".git/") || strings.HasPrefix(p, ".sky/"):
		return "", fmt.Errorf("«%s»: служебная папка не может быть scope", raw)
	}
	return p + "/", nil
}
