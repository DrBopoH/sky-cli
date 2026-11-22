// Package conf — формат .sky/sky.conf: строки «ключ = значение», списки в [a, b], секции [папка] с командами, комментарии с «#».
package conf

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/license"
)

const (
	DefaultScope = "project/"
	DefaultInbox = "~/Downloads"
)

// Commands — команды папки scope; пустая строка — не задана.
type Commands struct {
	Test  string
	Build string
}

// Get возвращает команду по имени («test», «build»).
func (c Commands) Get(kind string) string {
	switch kind {
	case "test":
		return c.Test
	case "build":
		return c.Build
	}
	return ""
}

// License — секция [license]: либо Type (встроенный текст с подстановкой), либо URL (своя лицензия по ссылке).
type License struct {
	Type   string
	URL    string
	Holder string
	Year   string // пусто — текущий год на момент создания LICENSE
	Email  string
}

// Fields — значения для подстановки в текст.
func (l License) Fields(defaultYear string) license.Fields {
	y := l.Year
	if y == "" {
		y = defaultYear
	}
	return license.Fields{Year: y, Holder: l.Holder, Email: l.Email}
}

type Conf struct {
	Scope      []string            // папки проекта от корня репозитория
	Inbox      string              // откуда sky забирает patch*.zip
	Regulation string              // необязательная ссылка на регламент; sky её не загружает
	Cmd        map[string]Commands // команды по папкам scope
	License    *License            // nil — секции [license] нет
}

func Default() Conf {
	return Conf{Scope: []string{DefaultScope}, Inbox: DefaultInbox}
}

const header = `# sky.conf — пути и команды проекта.
# scope — папки проекта от корня репозитория; sky применяет патчи, снимает «до» и коммитит только в них.
#   бинарь, одна папка:  scope = [project/]
#   веб:                 scope = [backend/, frontend/]
# inbox — папка, куда приходят patch*.zip.
# regulation — необязательная ссылка (URL) на регламент версий; sky её только хранит и не загружает.
# Секция [папка/] задаёт команды test и build этой папки (их запускает команда сборки sky); папка должна входить в scope:
#   [project/]
#   test = go test ./...
#   build = go build -o bin/app .
# Секция [license] — лицензия проекта; sky init создаёт по ней файл LICENSE, если его нет:
#   [license]
#   type = MIT
#   holder = Имя автора или компании
#   year = ГГГГ
#   email = name@example.com
# type — MIT, BSD-3-Clause, ISC или Unlicense; holder нужен всем, кроме Unlicense; year необязателен (по умолчанию текущий год);
# email необязателен и добавляется после holder. Вместо type можно дать url = https://… — свою лицензию по ссылке: файл не создаётся.
`

func Render(c Conf) string {
	reg := "# regulation = https://"
	if c.Regulation != "" {
		reg = "regulation = " + c.Regulation
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nscope = [%s]\ninbox = %s\n%s\n", header, strings.Join(c.Scope, ", "), c.Inbox, reg)
	for _, s := range c.Scope {
		cmd := c.Cmd[s]
		if cmd.Test == "" && cmd.Build == "" {
			continue
		}
		fmt.Fprintf(&b, "\n[%s]\n", s)
		if cmd.Test != "" {
			fmt.Fprintf(&b, "test = %s\n", cmd.Test)
		}
		if cmd.Build != "" {
			fmt.Fprintf(&b, "build = %s\n", cmd.Build)
		}
	}
	if l := c.License; l != nil {
		b.WriteString("\n[license]\n")
		for _, kv := range [][2]string{{"type", l.Type}, {"url", l.URL}, {"holder", l.Holder}, {"year", l.Year}, {"email", l.Email}} {
			if kv[1] != "" {
				fmt.Fprintf(&b, "%s = %s\n", kv[0], kv[1])
			}
		}
	}
	return b.String()
}

// Parse читает sky.conf; пропущенные ключи берутся по умолчанию, неизвестные — ошибка.
func Parse(src string) (Conf, error) {
	c := Default()
	section := ""
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		at := func(err error) error { return fmt.Errorf("sky.conf:%d: %w", i+1, err) }
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			if name == "license" {
				section = "license"
				if c.License == nil {
					c.License = &License{}
				}
				continue
			}
			if strings.ContainsAny(name, ", \t") {
				return c, at(fmt.Errorf("секция — одна папка, а не список"))
			}
			if !strings.HasSuffix(name, "/") {
				return c, at(fmt.Errorf("секция папки пишется со слэшем: [%s/] (без слэша бывают только ключевые секции, сейчас — [license])", name))
			}
			p, err := normPath(name)
			if err != nil {
				return c, at(err)
			}
			section = p
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return c, at(fmt.Errorf("нужен формат «ключ = значение»"))
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if section == "license" {
			if val == "" {
				return c, at(fmt.Errorf("%s пуст", key))
			}
			switch key {
			case "type":
				c.License.Type = val
			case "url":
				if !strings.HasPrefix(val, "https://") && !strings.HasPrefix(val, "http://") {
					return c, at(fmt.Errorf("url — ссылка вида https://…"))
				}
				c.License.URL = val
			case "holder":
				c.License.Holder = val
			case "year":
				if len(val) != 4 || strings.Trim(val, "0123456789") != "" {
					return c, at(fmt.Errorf("year — четыре цифры"))
				}
				c.License.Year = val
			case "email":
				c.License.Email = val
			default:
				return c, at(fmt.Errorf("в секции [license] есть только type, url, holder, year и email, а не «%s»", key))
			}
			continue
		}
		if section != "" {
			cmd := c.Cmd[section]
			switch key {
			case "test":
				cmd.Test = val
			case "build":
				cmd.Build = val
			default:
				return c, at(fmt.Errorf("в секции [%s] есть только test и build, а не «%s»", section, key))
			}
			if val == "" {
				return c, at(fmt.Errorf("%s пуст", key))
			}
			if c.Cmd == nil {
				c.Cmd = map[string]Commands{}
			}
			c.Cmd[section] = cmd
			continue
		}
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
		case "regulation":
			if !strings.HasPrefix(val, "https://") && !strings.HasPrefix(val, "http://") {
				return c, at(fmt.Errorf("regulation — ссылка вида https://…"))
			}
			c.Regulation = val
		default:
			return c, at(fmt.Errorf("неизвестный ключ «%s»", key))
		}
	}
	if l := c.License; l != nil {
		switch {
		case l.Type != "" && l.URL != "":
			return c, fmt.Errorf("sky.conf: в [license] нужно одно из двух: type или url")
		case l.Type == "" && l.URL == "":
			return c, fmt.Errorf("sky.conf: в [license] не задан ни type, ни url")
		case l.Type != "":
			if err := license.Check(l.Type, l.Fields("2000")); err != nil {
				return c, fmt.Errorf("sky.conf: [license]: %w", err)
			}
		case l.Holder != "" || l.Year != "" || l.Email != "":
			return c, fmt.Errorf("sky.conf: holder, year и email в [license] работают только с type")
		}
	}
	for s := range c.Cmd {
		if !c.InScope(s) {
			return c, fmt.Errorf("sky.conf: секция [%s] — такой папки нет в scope", s)
		}
	}
	return c, nil
}

func (c Conf) InScope(p string) bool {
	for _, s := range c.Scope {
		if s == p {
			return true
		}
	}
	return false
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
