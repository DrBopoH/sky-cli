// Package fix — исправления сборки по регламенту (3.6): фича, ветвившаяся от ветки сборки, одним squash-коммитом
// ложится на ветку сборки с версией vR.sB-F.C+T.hash: F растёт на 1, C остаётся как у сборки, T растёт
// на 1 от всех именованных коммитов и исправлений текущего R. В dev исправление не вливается.
package fix

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/named"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Dev    = "dev"
	MaxLen = 72
)

// Options — параметры одного вызова.
type Options struct {
	Root    string
	Summary string
	Build   string // ветка сборки («build/1125-0336» или «1125-0336»); пусто — самая поздняя
	Dry     bool
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines   []string
	Message string
	Branch  string // ветка, на которой остался HEAD
	Target  string // ветка сборки
}

// Run выполняет fix по параметрам.
func Run(o Options) (Result, error) {
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	err := run(o, &res, say)
	res.Branch = strings.TrimSpace(gitx.Try(o.Root, "symbolic-ref", "-q", "--short", "HEAD"))
	return res, err
}

func run(o Options, res *Result, say func(string, ...any)) error {
	s := o.Summary
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("нужно резюме: sky fix \"summary\"")
	case strings.ContainsAny(s, "\r\n"):
		return fmt.Errorf("резюме одной строкой")
	case utf8.RuneCountInString(s) > MaxLen:
		return fmt.Errorf("резюме длиннее %d символов", MaxLen)
	}
	if d := gitx.Dirty(o.Root); d != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: закоммить (sky commit) или убери\n%s", d)
	}
	cur := strings.TrimSpace(gitx.Try(o.Root, "symbolic-ref", "-q", "--short", "HEAD"))
	switch {
	case cur == "":
		return fmt.Errorf("HEAD не на ветке: перейди на ветку исправления")
	case cur == Dev:
		return fmt.Errorf("исправление пишется на отдельной ветке, а не в %s: создай её от ветки сборки", Dev)
	case strings.HasPrefix(cur, vhist.Prefix):
		return fmt.Errorf("ты на ветке сборки %s: перейди на ветку исправления, отведённую от неё", cur)
	}
	h := vhist.H{Root: o.Root}
	target, head, err := h.Pick(o.Build)
	if err != nil {
		return err
	}
	if !gitx.OK(o.Root, "merge-base", "--is-ancestor", "refs/heads/"+target, "refs/heads/"+cur) {
		return fmt.Errorf("ветка %s не отведена от %s: создай ветку исправления от ветки сборки (git checkout -b имя %s)", cur, target, target)
	}
	if strings.TrimSpace(gitx.Try(o.Root, "rev-list", "--count", target+".."+cur)) == "0" {
		return fmt.Errorf("в %s нет коммитов, которых нет в %s", cur, target)
	}
	t := head.T
	if dev, found, err := h.Last(Dev); err == nil && found && dev.R == head.R && dev.T > t {
		t = dev.T
	}
	if m := h.MaxT(head.R); m > t {
		t = m
	}
	v := named.Version{
		Full: true, R: head.R, B: head.B, C: head.C, T: t + 1,
		State: head.State, F: head.F + 1,
		Hash: strings.TrimSpace(gitx.Try(o.Root, "rev-parse", "--short=7", "refs/heads/"+cur)),
	}
	msg := named.Message(s, v)
	res.Message, res.Target = msg, target
	say("исправление: %s → %s (squash)", cur, target)
	say("  %s", msg)
	if o.Dry {
		return nil
	}
	return gitx.SquashInto(o.Root, target, cur, msg, cur)
}
