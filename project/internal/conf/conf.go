// Package conf — формат .sky/sky.conf: строки «ключ = значение», списки в [a, b], комментарии с «#».
package conf

import (
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	DefaultScope = "project/"
	DefaultInbox = "~/Downloads"
)

// BuildPoint — одна точка сборки: папка от корня репозитория и команда, которая в ней запускается.
type BuildPoint struct {
	Dir string // нормализованная, с «/» на конце; «./» — корень репозитория
	Cmd string
}

type Conf struct {
	Scope      []string     // папки проекта от корня репозитория
	Build      []BuildPoint // точки сборки по порядку; пусто — команд сборки нет
	Inbox      string       // откуда sky забирает patch*.zip
	Regulation string       // необязательная ссылка на регламент; sky её не загружает
}

func Default() Conf {
	return Conf{Scope: []string{DefaultScope}, Inbox: DefaultInbox}
}

//go:embed text/header.txt
var header string

func Render(c Conf) string {
	reg := "# regulation = https://"
	if c.Regulation != "" {
		reg = "regulation = " + c.Regulation
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nscope = [%s]\ninbox = %s\n%s\n", header, strings.Join(c.Scope, ", "), c.Inbox, reg)
	for _, p := range c.Build {
		fmt.Fprintf(&b, "build = %s %s\n", p.Dir, p.Cmd)
	}
	return b.String()
}

// Parse читает sky.conf; пропущенные ключи берутся по умолчанию, неизвестные — ошибка.
func Parse(src string) (Conf, error) {
	c := Default()
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		at := func(err error) error { return fmt.Errorf("sky.conf:%d: %w", i+1, err) }
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return c, at(fmt.Errorf("секций в sky.conf нет"))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return c, at(fmt.Errorf("нужен формат «ключ = значение»"))
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "scope":
			s, err := ParseScope([]string{val})
			if err != nil {
				return c, at(err)
			}
			c.Scope = s
		case "inbox":
			if val == "" {
				return c, at(fmt.Errorf("inbox пуст"))
			}
			c.Inbox = val
		case "build":
			dir, cmd, ok := strings.Cut(val, " ")
			cmd = strings.TrimSpace(cmd)
			if !ok || cmd == "" {
				return c, at(fmt.Errorf("build = <папка> <команда>, например build = backend/ go build ./..."))
			}
			d, err := normPath(dir)
			if err != nil {
				return c, at(err)
			}
			c.Build = append(c.Build, BuildPoint{Dir: d, Cmd: cmd})
		case "regulation":
			if !strings.HasPrefix(val, "https://") && !strings.HasPrefix(val, "http://") {
				return c, at(fmt.Errorf("regulation — ссылка вида https://…"))
			}
			c.Regulation = val
		default:
			return c, at(fmt.Errorf("неизвестный ключ «%s»", key))
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

// Load читает файл конфига.
func Load(file string) (Conf, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return Default(), err
	}
	return Parse(string(b))
}

// InboxDir раскрывает «~» в inbox.
func (c Conf) InboxDir() (string, error) {
	p := c.Inbox
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p, nil
}
