// Package repo — раскладка .sky/ в репозитории и запись файлов, которыми владеет sky.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// legacyFiles — файлы прежней раскладки, которые sky при проверке удаляет.
var legacyFiles = []string{"clouds/sky-reglament.md"}

type Layout struct {
	Root   string // корень git-репозитория
	Dir    string // .sky/
	Conf   string // .sky/sky.conf
	Stages string // .sky/stages/
	Readme string // .sky/readme/ — неизменяемое, создаёт sky
	Foryou string // .sky/foryou/ — сгенерированное, создаёт sky
	Clouds string // .sky/clouds/ — единственное место для записей Claude
}

func Find(cwd string) (Layout, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return Layout{}, errors.New("это не git-репозиторий")
	}
	return At(strings.TrimSpace(string(out))), nil
}

func At(root string) Layout {
	dir := filepath.Join(root, ".sky")
	return Layout{
		Root:   root,
		Dir:    dir,
		Conf:   filepath.Join(dir, "sky.conf"),
		Stages: filepath.Join(dir, "stages"),
		Readme: filepath.Join(dir, "readme"),
		Foryou: filepath.Join(dir, "foryou"),
		Clouds: filepath.Join(dir, "clouds"),
	}
}

func (l Layout) Initialized() bool {
	st, err := os.Stat(l.Dir)
	return err == nil && st.IsDir()
}

func (l Layout) MkDirs() error {
	for _, d := range []string{l.Stages, l.Readme, l.Foryou, l.Clouds} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

type Status int

const (
	Same Status = iota
	Created
	Replaced
	Removed
)

type Change struct {
	Path   string // от корня репозитория
	Status Status
}

type Generated struct {
	Name    string
	Content []byte
}

// SyncReadme приводит .sky/readme/ к заданному набору: недостающее создаёт, изменённое перезаписывает (0444),
// лишнее удаляет. Заодно убирает файлы прежней раскладки.
func (l Layout) SyncReadme(files []Generated) ([]Change, error) {
	if err := l.MkDirs(); err != nil {
		return nil, err
	}
	var out []Change
	want := map[string]bool{}
	for _, f := range files {
		want[f.Name] = true
		st, err := writeReadOnly(filepath.Join(l.Readme, f.Name), l.Readme, f.Content)
		if err != nil {
			return out, err
		}
		out = append(out, Change{l.rel(filepath.Join(l.Readme, f.Name)), st})
	}
	entries, err := os.ReadDir(l.Readme)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if want[e.Name()] {
			continue
		}
		p := filepath.Join(l.Readme, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return out, err
		}
		out = append(out, Change{l.rel(p), Removed})
	}
	for _, name := range legacyFiles {
		p := filepath.Join(l.Dir, filepath.FromSlash(name))
		if _, err := os.Lstat(p); err == nil {
			if err := os.Remove(p); err != nil {
				return out, err
			}
			out = append(out, Change{l.rel(p), Removed})
		}
	}
	return out, nil
}

func (l Layout) rel(p string) string {
	r, err := filepath.Rel(l.Root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

func writeReadOnly(path, dir string, content []byte) (Status, error) {
	cur, err := os.ReadFile(path)
	st := Created
	if err == nil {
		st = Replaced
		if bytes.Equal(cur, content) {
			if fi, e := os.Stat(path); e == nil && fi.Mode().Perm() == 0o444 {
				return Same, nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Same, fmt.Errorf("%s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return Same, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return Same, err
	}
	if err := tmp.Chmod(0o444); err != nil {
		tmp.Close()
		return Same, err
	}
	if err := tmp.Close(); err != nil {
		return Same, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Same, err
	}
	return st, nil
}

// EnsureIgnored добавляет строку в .gitignore, если её там ещё нет; возвращает true, если дописала.
func (l Layout) EnsureIgnored(line string) (bool, error) {
	p := filepath.Join(l.Root, ".gitignore")
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	bare := strings.Trim(line, "/")
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.Trim(strings.TrimSpace(ln), "/") == bare {
			return false, nil
		}
	}
	s := string(b)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return true, os.WriteFile(p, []byte(s+line+"\n"), 0o644)
}
