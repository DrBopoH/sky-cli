// Package conf — формат .sky/sky.conf: строки «ключ = значение», списки в [a, b], секции [папка] с командами, комментарии с «#».
package conf

import (
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

type Conf struct {
	Scope      []string            // папки проекта от корня репозитория
	Inbox      string              // откуда sky забирает patch*.zip
	Regulation string              // необязательная ссылка на регламент; sky её не загружает
	Cmd        map[string]Commands // команды по папкам scope
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
# Секция [папка] задаёт команды этой папки; sky test и sky build запускают их из неё:
#   [project/]
#   test = go test ./...
#   build = go build -o bin/app .
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
			if strings.ContainsAny(name, ", \t") {
				return c, at(fmt.Errorf("секция — одна папка, а не список"))
			}
			if !strings.HasSuffix(name, "/") {
				return c, at(fmt.Errorf("секция папки пишется со слэшем: [%s/]", name))
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
