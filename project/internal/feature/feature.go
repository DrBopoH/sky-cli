// Package feature заводит локальную ветку фичи по регламенту (3.1): wip/ММДД-ЧЧММ/имя.
// Цель ветки (модуль с цепочкой, зерно или исправление сборки) запоминается в git config:
// sky land и sky fix на этой ветке берут её оттуда и обходятся без флагов.
package feature

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/graph"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Prefix = "wip/"
	Dev    = "dev"
	maxLen = 40
)

// Kind — куда ветка попадёт.
type Kind string

const (
	Module Kind = "module" // squash в ветку модуля
	Seed   Kind = "seed"   // squash в dev (изменение зерна)
	Fix    Kind = "fix"    // squash в ветку сборки
)

// Target — запомненная цель ветки.
type Target struct {
	Kind   Kind
	Module string
	Chain  []string
	Base   string // ветка, от которой отведена (для Fix — ветка сборки)
}

// Options — параметры одного вызова.
type Options struct {
	Root   string
	Name   string
	Module string
	Chain  []string
	Seed   bool
	Fix    bool
	Build  string // с Fix: ветка сборки; пусто — самая поздняя
	Dry    bool
	Now    time.Time
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines  []string
	Branch string
	Base   string
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func cfg(root, branch, key string) string { return gitx.BranchConfig(root, branch, key) }

// Load возвращает цель ветки; ok=false, если sky её не заводил.
func Load(root, branch string) (Target, bool) {
	k := Kind(cfg(root, branch, "sky-target"))
	if k != Module && k != Seed && k != Fix {
		return Target{}, false
	}
	t := Target{Kind: k, Module: cfg(root, branch, "sky-module"), Base: cfg(root, branch, "sky-base")}
	if c := cfg(root, branch, "sky-chain"); c != "" {
		t.Chain = strings.Split(c, ",")
	}
	return t, true
}

// Current — текущая ветка и её цель.
func Current(root string) (string, Target, bool) {
	b := strings.TrimSpace(gitx.Try(root, "symbolic-ref", "-q", "--short", "HEAD"))
	if b == "" {
		return "", Target{}, false
	}
	t, ok := Load(root, b)
	return b, t, ok
}

// Run создаёт ветку и переключается на неё.
func Run(o Options) (Result, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	err := run(o, &res, say)
	return res, err
}

func run(o Options, res *Result, say func(string, ...any)) error {
	switch {
	case !nameRe.MatchString(o.Name) || len(o.Name) > maxLen:
		return fmt.Errorf("имя фичи: латиница в нижнем регистре, цифры и дефис, до %d символов (например add-cache)", maxLen)
	case o.Fix && (o.Seed || o.Module != "" || len(o.Chain) > 0):
		return fmt.Errorf("--fix несовместим с --module, --chain и --seed")
	case o.Seed && (o.Module != "" || len(o.Chain) > 0):
		return fmt.Errorf("--seed несовместим с --module и --chain")
	case len(o.Chain) > 0 && o.Module == "":
		return fmt.Errorf("--chain задаётся вместе с --module")
	case o.Build != "" && !o.Fix:
		return fmt.Errorf("--build задаётся вместе с --fix")
	}
	if d := gitx.Dirty(o.Root); d != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: закоммить (sky commit) или убери\n%s", d)
	}
	if !gitx.OK(o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+Dev) {
		return fmt.Errorf("нет ветки %s", Dev)
	}
	t := Target{Base: Dev}
	switch {
	case o.Fix:
		h := vhist.H{Root: o.Root}
		if o.Build == "" {
			b, _, ok := h.Latest()
			if !ok {
				return fmt.Errorf("сборок нет: исправлять нечего (сначала sky build)")
			}
			t.Base = b
		} else {
			b := o.Build
			if !strings.HasPrefix(b, "build/") {
				b = "build/" + b
			}
			if _, ok := h.Head(b); !ok {
				return fmt.Errorf("«%s» не ветка сборки (нужна «build/ММДД-ЧЧММ»)", o.Build)
			}
			t.Base = b
		}
		t.Kind = Fix
	case o.Seed:
		t.Kind = Seed
	case o.Module != "":
		g, err := graph.Load(o.Root)
		if err != nil {
			return err
		}
		chain, err := g.Resolve(o.Module, o.Chain)
		if err != nil {
			return err
		}
		t.Kind, t.Module, t.Chain = Module, o.Module, chain
	}
	name := Prefix + o.Now.Format("0102-1504") + "/" + o.Name
	if gitx.OK(o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+name) {
		return fmt.Errorf("ветка %s уже есть", name)
	}
	res.Branch, res.Base = name, t.Base
	say("ветка %s от %s", name, t.Base)
	switch t.Kind {
	case Module:
		say("  цель: модуль %s, цепочка %s → %s", t.Module, graph.Path, strings.Join(t.Chain, " → "))
	case Seed:
		say("  цель: зерно (squash в %s)", Dev)
	case Fix:
		say("  цель: исправление сборки %s", t.Base)
	default:
		hint(o, say)
	}
	if o.Dry {
		return nil
	}
	if _, err := gitx.Run(o.Root, "checkout", "-q", "-b", name, t.Base); err != nil {
		return err
	}
	if t.Kind == "" {
		return nil
	}
	set := func(key, val string) error {
		_, err := gitx.Run(o.Root, "config", "branch."+name+"."+key, val)
		return err
	}
	if err := set("sky-target", string(t.Kind)); err != nil {
		return err
	}
	if err := set("sky-base", t.Base); err != nil {
		return err
	}
	if t.Module != "" {
		if err := set("sky-module", t.Module); err != nil {
			return err
		}
		return set("sky-chain", strings.Join(t.Chain, ","))
	}
	return nil
}

// hint подсказывает цель, если она не задана: модули из реестра и те, что похожи на имя фичи.
func hint(o Options, say func(string, ...any)) {
	say("  цель не задана: на sky land понадобится --module M или --seed")
	g, err := graph.Load(o.Root)
	if err != nil || len(g.Order) == 0 {
		return
	}
	var like []string
	for _, tok := range strings.Split(o.Name, "-") {
		for _, m := range g.Order {
			if strings.EqualFold(m, tok) {
				like = append(like, m)
			}
		}
	}
	if len(like) > 0 {
		say("  похоже на модуль: sky feature %s --module %s", o.Name, like[0])
	}
	list := g.Order
	more := ""
	if len(list) > 12 {
		list, more = list[:12], fmt.Sprintf(" … ещё %d", len(g.Order)-12)
	}
	say("  модули проекта: %s%s", strings.Join(list, ", "), more)
}
