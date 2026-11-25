// Package repo — раскладка .sky/ в репозитории и запись файлов, которыми владеет sky.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
)

type Layout struct {
	Root    string // корень git-репозитория
	Dir     string // .sky/
	Conf    string // .sky/sky.conf
	Stages  string // .sky/stages/
	History string // .sky/stages/history/ — записи применённых патчей и откатов
	Backups string // .sky/stages/backups/ — полные снимки проекта перед патчами
	Readme  string // .sky/readme/ — неизменяемое, создаёт sky
	Foryou  string // .sky/foryou/ — сгенерированное, создаёт sky
	Clouds  string // .sky/clouds/ — единственное место для записей Claude
}

func Find(cwd string) (Layout, error) {
	top, err := gitx.Toplevel(cwd)
	if err != nil {
		return Layout{}, err
	}
	return At(top), nil
}

// FindOrInit находит репозиторий; если cwd не внутри git, создаёт его (git init, ветка main). created — создан ли.
func FindOrInit(cwd string) (l Layout, created bool, err error) {
	if l, err = Find(cwd); err == nil {
		return l, false, nil
	}
	if err = gitx.Init(cwd); err != nil {
		return Layout{}, false, err
	}
	l, err = Find(cwd)
	return l, err == nil, err
}

func At(root string) Layout {
	dir := filepath.Join(root, ".sky")
	return Layout{
		Root:    root,
		Dir:     dir,
		Conf:    filepath.Join(dir, "sky.conf"),
		Stages:  filepath.Join(dir, "stages"),
		History: filepath.Join(dir, "stages", "history"),
		Backups: filepath.Join(dir, "stages", "backups"),
		Readme:  filepath.Join(dir, "readme"),
		Foryou:  filepath.Join(dir, "foryou"),
		Clouds:  filepath.Join(dir, "clouds"),
	}
}

func (l Layout) Initialized() bool {
	st, err := os.Stat(l.Dir)
	return err == nil && st.IsDir()
}

func (l Layout) MkDirs() error {
	for _, d := range []string{l.History, l.Backups, l.Readme, l.Foryou, l.Clouds} {
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

// CanCommit сообщает, знает ли git имя и почту автора для коммита в dir.
func CanCommit(dir string) bool {
	return gitx.CanCommit(dir)
}

// HasCommits сообщает, есть ли в репозитории хотя бы один коммит (HEAD указывает на существующий).
func (l Layout) HasCommits() bool {
	return gitx.HasCommits(l.Root)
}

// FirstCommit создаёт первый коммит из перечисленных файлов (если файлов нет — пустой), затем ветку dev от него
// и переключается на неё. Вызывать только в репозитории без коммитов.
func (l Layout) FirstCommit(msg string, files []string) error {
	run := func(args ...string) error {
		_, err := gitx.Run(l.Root, args...)
		return err
	}
	if len(files) > 0 {
		if err := run(append([]string{"add", "--"}, files...)...); err != nil {
			return err
		}
	}
	if err := run("commit", "-q", "--allow-empty", "-m", msg); err != nil {
		return err
	}
	return run("switch", "-q", "-c", "dev")
}

// DevState — что EnsureDev сделал с веткой dev.
type DevState int

const (
	DevExists  DevState = iota // dev уже есть, ничего не менялось
	DevCreated                 // dev создана от вершины main/master и активна
	DevSkipped                 // dev нет, но HEAD не на main/master: откуда ей идти, решает человек
)

// mainBranches — ветки, от вершины которых sky сам отводит dev в репозитории с историей.
var mainBranches = map[string]bool{"main": true, "master": true}

// EnsureDev заводит dev в репозитории с историей: от вершины main (или master), если HEAD стоит на ней.
// Рабочее дерево и история не меняются; cur — ветка HEAD («» при detached HEAD).
func (l Layout) EnsureDev() (state DevState, cur string, err error) {
	if gitx.OK(l.Root, "rev-parse", "--verify", "-q", "refs/heads/dev") {
		return DevExists, "", nil
	}
	cur = strings.TrimSpace(gitx.Try(l.Root, "symbolic-ref", "-q", "--short", "HEAD"))
	if !mainBranches[cur] {
		return DevSkipped, cur, nil
	}
	if _, err := gitx.Run(l.Root, "switch", "-q", "-c", "dev"); err != nil {
		return DevSkipped, cur, err
	}
	return DevCreated, cur, nil
}

// SyncForyou приводит .sky/foryou/ к переданному набору: пишет файлы только для чтения, лишнее убирает.
func (l Layout) SyncForyou(files []Generated) ([]Change, error) {
	if err := l.MkDirs(); err != nil {
		return nil, err
	}
	var out []Change
	want := map[string]bool{}
	for _, f := range files {
		want[f.Name] = true
		p := filepath.Join(l.Foryou, f.Name)
		st, err := writeReadOnly(p, l.Foryou, f.Content)
		if err != nil {
			return out, err
		}
		out = append(out, Change{l.rel(p), st})
	}
	entries, err := os.ReadDir(l.Foryou)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if want[e.Name()] {
			continue
		}
		p := filepath.Join(l.Foryou, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return out, err
		}
		out = append(out, Change{l.rel(p), Removed})
	}
	return out, nil
}

// WriteForyouFile пишет один файл в foryou/ только для чтения.
func (l Layout) WriteForyouFile(name string, content []byte) error {
	if err := l.MkDirs(); err != nil {
		return err
	}
	_, err := writeReadOnly(filepath.Join(l.Foryou, name), l.Foryou, content)
	return err
}
