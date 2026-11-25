// Package publish — публикация сборки по регламенту (3.7): голова ветки сборки ложится в main одним коммитом.
// Коммит строится от дерева головы сборки поверх вершины main (git commit-tree), а не слиянием: так вторая
// публикация не упирается в конфликт с первой. Версия коммита — версия головы сборки, хэш — её последний коммит.
// Ветка сборки остаётся как есть, dev не трогается, HEAD не переключается.
package publish

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/named"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Main = "main"
)

// Options — параметры одного вызова.
type Options struct {
	Root  string
	Build string // «build/ММДД-ЧЧММ» или «ММДД-ЧЧММ»; пусто — самая поздняя сборка
	Title string // своё название вместо названия сборки
	Dry   bool
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines   []string
	Message string
	Build   string // опубликованная ветка сборки
	Done    bool
}

// Run выполняет публикацию по параметрам.
func Run(o Options) (Result, error) {
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	root := o.Root
	if !gitx.OK(root, "rev-parse", "--verify", "-q", "refs/heads/"+Main) {
		return res, fmt.Errorf("нет ветки %s", Main)
	}
	if cur := strings.TrimSpace(gitx.Try(root, "symbolic-ref", "-q", "--short", "HEAD")); cur == Main {
		return res, fmt.Errorf("HEAD на %s: перейди на другую ветку (например dev), публикация двигает %s, не трогая рабочее дерево", Main, Main)
	}
	h := vhist.H{Root: root}
	branch, head, err := h.Pick(o.Build)
	if err != nil {
		return res, err
	}
	res.Build = branch
	title := o.Title
	if strings.TrimSpace(title) == "" {
		if title = tailTitle(h, branch); title == "" {
			return res, fmt.Errorf("у ветки %s нет хвоста сборки: название не из чего взять, задай --title", branch)
		}
	}
	title = strings.TrimSpace(title)
	if err := named.CheckLine("название", title); err != nil {
		return res, err
	}
	tip := strings.TrimSpace(gitx.Try(root, "rev-parse", "--verify", "refs/heads/"+branch))
	head.Hash = strings.TrimSpace(gitx.Try(root, "rev-parse", "--short=7", tip))
	res.Message = named.Message(title, head)
	tree := strings.TrimSpace(gitx.Try(root, "rev-parse", tip+"^{tree}"))
	mainTip := strings.TrimSpace(gitx.Try(root, "rev-parse", "--verify", "refs/heads/"+Main))
	if tree == strings.TrimSpace(gitx.Try(root, "rev-parse", mainTip+"^{tree}")) {
		return res, fmt.Errorf("в %s уже лежит это дерево: сборка %s опубликована", Main, branch)
	}
	say("публикация %s → %s (один коммит)", branch, Main)
	say("  %s", res.Message)
	if o.Dry {
		return res, nil
	}
	commit, err := gitx.Run(root, "commit-tree", tree, "-p", mainTip, "-m", res.Message)
	if err != nil {
		return res, err
	}
	if _, err := gitx.Run(root, "update-ref", "-m", "sky publish", "refs/heads/"+Main, strings.TrimSpace(commit), mainTip); err != nil {
		return res, err
	}
	res.Done = true
	say("✓ %s: %s", Main, res.Message)
	return res, nil
}

// tailTitle — название из хвоста сборки, лежащего на ветке.
func tailTitle(h vhist.H, branch string) string {
	for _, b := range h.Builds() {
		if h.Branch(b) != branch {
			continue
		}
		s, _, ok := named.Parse(strings.TrimSpace(gitx.Try(h.Root, "log", "-1", "--format=%s", b.Hash)))
		if ok {
			return s
		}
	}
	return ""
}
