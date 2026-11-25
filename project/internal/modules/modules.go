// Package modules показывает реестр модулей проекта (docs/PROJECT-GRAPH.md) вместе с открытыми ветками модулей:
// уровни, зависимости, цепочки от зерна, ветки «модуль/ММДД-ЧЧММ» и их состояние. Ничего не меняет.
package modules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/graph"
)

var branchRe = regexp.MustCompile(`^([^/]+)/(\d{4}-\d{4})$`)

// reserved — первые слова веток, которые не относятся к модулям.
var reserved = map[string]bool{"build": true, "wip": true}

// ParseBranch разбирает имя «модуль/ММДД-ЧЧММ».
func ParseBranch(name string) (module, stamp string, ok bool) {
	m := branchRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// Branch — ветка модуля со своим состоянием.
type Branch struct {
	Name   string
	Module string
	State  string // open, merged; пусто — ветка заведена не sky land
	Parent string
}

// Branches — ветки модулей репозитория (сборки и wip не входят), по имени.
func Branches(root string) []Branch {
	var out []Branch
	for _, n := range strings.Split(strings.TrimSpace(gitx.Try(root, "for-each-ref", "--format=%(refname:short)", "refs/heads/")), "\n") {
		m, _, ok := ParseBranch(n)
		if !ok || reserved[m] {
			continue
		}
		out = append(out, Branch{Name: n, Module: m, State: gitx.BranchConfig(root, n, "sky-state"), Parent: gitx.BranchConfig(root, n, "sky-parent")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Result — отчёт.
type Result struct {
	Lines []string
	Warn  int // сколько замечаний в отчёте
}

type report struct {
	res *Result
}

func (r *report) say(f string, a ...any) { r.res.Lines = append(r.res.Lines, fmt.Sprintf(f, a...)) }
func (r *report) warn(f string, a ...any) {
	r.res.Warn++
	r.say("⚠ "+f, a...)
}

// Inspect строит отчёт: без module — весь реестр, с module — один модуль.
func Inspect(root, module string) (Result, error) {
	res := Result{}
	r := &report{res: &res}
	g, err := graph.Load(root)
	if err != nil {
		return res, err
	}
	seed, err := g.Root()
	if err != nil {
		return res, err
	}
	brs := Branches(root)
	if module != "" {
		if _, ok := g.Deps[module]; !ok {
			return res, fmt.Errorf("модуля «%s» нет в графе проекта (sky modules без аргумента покажет все)", module)
		}
		r.one(g, seed, module, brs)
		return res, nil
	}
	r.all(g, seed, brs)
	return res, nil
}

func (r *report) all(g graph.Graph, seed string, brs []Branch) {
	r.say("модулей: %d, зерно: %s", len(g.Order), seed)
	levels := g.ByLevel()
	for i := len(levels) - 1; i >= 0; i-- {
		r.say("уровень %d", i)
		for _, m := range levels[i] {
			mark := ""
			if m == seed {
				mark = "  (зерно)"
			}
			r.say("  %-12s зависит от: %s%s", m, deps(g.Deps[m]), mark)
		}
	}
	r.branches(g, brs)
}

func (r *report) one(g graph.Graph, seed, m string, brs []Branch) {
	r.say("%s — уровень %d", m, g.Levels()[m])
	r.say("  зависит от: %s", deps(g.Deps[m]))
	r.say("  от него зависят: %s", deps(g.Dependents(m)))
	if m == seed {
		r.say("  зерно: правки идут флагом --seed прямо в dev")
	} else if cs, err := g.Chains(m); err == nil {
		r.say("  цепочки от зерна (--chain):")
		for _, c := range cs {
			r.say("    dev → %s", strings.Join(c, " → "))
		}
	}
	var mine []Branch
	for _, b := range brs {
		if b.Module == m {
			mine = append(mine, b)
		}
	}
	r.list(g, mine)
}

func (r *report) branches(g graph.Graph, brs []Branch) {
	if len(brs) == 0 {
		r.say("веток модулей нет")
		return
	}
	r.say("ветки модулей:")
	r.list(g, brs)
}

func (r *report) list(g graph.Graph, brs []Branch) {
	if len(brs) == 0 {
		r.say("  открытых веток нет")
		return
	}
	for _, b := range brs {
		state := b.State
		if state == "" {
			state = "заведена не sky land"
		}
		line := fmt.Sprintf("  %s  [%s]", b.Name, state)
		if b.Parent != "" {
			line += " → " + b.Parent
		}
		r.say("%s", line)
		if _, ok := g.Deps[b.Module]; !ok {
			r.warn("«%s» нет в графе проекта: ветка %s ни к какому модулю не относится", b.Module, b.Name)
		}
	}
}

func deps(ds []string) string {
	if len(ds) == 0 {
		return "—"
	}
	return strings.Join(ds, ", ")
}
