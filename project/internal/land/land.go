// Package land переносит локальную фичу в удалённый граф по регламенту, но выполняет это локально (сценарий 7):
// фича → ветка модуля (squash), ветка модуля → вышележащая ветка или dev (merge-коммит), изменение зерна → dev (squash).
// Версию именованного коммита считает по истории dev (регламент, 3.5). Ничего не пушит и не удаляет.
package land

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/graph"
	"github.com/DrBopoH/sky-cli/internal/named"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Dev       = "dev"
	GraphPath = "docs/PROJECT-GRAPH.md"
	MaxLen    = 72
)

// Options — параметры одного вызова.
type Options struct {
	Root    string
	Summary string
	Module  string   // фича → ветка этого модуля
	Chain   []string // цепочка от зерна вниз до модуля; пусто — по графу, если путь один
	Seed    bool     // изменение зерна прямо в dev
	Dry     bool     // только описать, ничего не менять
	From    string   // «R.B.C+T»: точка отсчёта, пока в dev нет именованных коммитов
	Now     time.Time
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines   []string
	Message string
	Branch  string // ветка, на которой остался HEAD
}

var branchRe = regexp.MustCompile(`^([^/]+)/(\d{4}-\d{4})$`)

type env struct {
	o   Options
	res *Result
}

func (e *env) say(f string, a ...any)             { e.res.Lines = append(e.res.Lines, fmt.Sprintf(f, a...)) }
func (e *env) git(args ...string) (string, error) { return gitx.Run(e.o.Root, args...) }
func (e *env) try(args ...string) string          { return strings.TrimSpace(gitx.Try(e.o.Root, args...)) }
func (e *env) cfg(branch, key string) string {
	return e.try("config", "--get", "branch."+branch+"."+key)
}

func (e *env) setCfg(branch, key, val string) error {
	_, err := e.git("config", "branch."+branch+"."+key, val)
	return err
}

func (e *env) exists(branch string) bool {
	return gitx.OK(e.o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+branch)
}

func (e *env) short(rev string) string { return e.try("rev-parse", "--short=7", rev) }

// lastNamed — версия, от которой считается следующее изменение (vhist.Effective).
func (e *env) lastNamed(rev string) (named.Version, bool, error) {
	return vhist.H{Root: e.o.Root, From: e.o.From}.Effective(rev)
}

func (e *env) count(from, to string) int {
	n, _ := strconv.Atoi(e.try("rev-list", "--count", "-E", "--grep="+named.Grep, from+".."+to))
	return n
}

func (e *env) current() (string, error) {
	b := e.try("symbolic-ref", "-q", "--short", "HEAD")
	if b == "" {
		return "", fmt.Errorf("HEAD не на ветке: перейди на ветку фичи или модуля")
	}
	return b, nil
}

func (e *env) checkout(b string) error {
	_, err := e.git("checkout", "-q", b)
	return err
}

type step struct {
	name, parent string
	create       bool
}

// open — открытая ветка модуля с нужным родителем.
func (e *env) findOpen(module, parent string) string {
	out := e.try("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	for _, b := range strings.Split(out, "\n") {
		m := branchRe.FindStringSubmatch(b)
		if m == nil || m[1] != module {
			continue
		}
		if e.cfg(b, "sky-state") == "open" && e.cfg(b, "sky-parent") == parent {
			return b
		}
	}
	return ""
}

func (e *env) plan(chain []string) ([]step, error) {
	stamp := e.o.Now.Format("0102-1504")
	parent := Dev
	var steps []step
	for _, m := range chain {
		if b := e.findOpen(m, parent); b != "" {
			steps = append(steps, step{name: b, parent: parent})
			parent = b
			continue
		}
		name := m + "/" + stamp
		if e.exists(name) {
			return nil, fmt.Errorf("ветка %s уже есть и не открыта для этой цепочки: подожди минуту или переименуй её", name)
		}
		steps = append(steps, step{name: name, parent: parent, create: true})
		parent = name
	}
	return steps, nil
}

func (e *env) validate() error {
	s := e.o.Summary
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("нужно резюме: sky land \"summary\"")
	case strings.ContainsAny(s, "\r\n"):
		return fmt.Errorf("резюме одной строкой")
	case utf8.RuneCountInString(s) > MaxLen:
		return fmt.Errorf("резюме длиннее %d символов", MaxLen)
	}
	if d := gitx.Dirty(e.o.Root); d != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: закоммить (sky commit) или убери\n%s", d)
	}
	if !e.exists(Dev) {
		return fmt.Errorf("нет ветки %s", Dev)
	}
	return nil
}

func (e *env) loadGraph() (graph.Graph, error) {
	b, err := os.ReadFile(filepath.Join(e.o.Root, filepath.FromSlash(GraphPath)))
	if err != nil {
		return graph.Graph{}, fmt.Errorf("нет графа проекта %s: его ведёт агент (см. .sky/readme/readme.md)", GraphPath)
	}
	return graph.Parse(string(b))
}

func (e *env) resolveChain(g graph.Graph) ([]string, error) {
	if len(e.o.Chain) > 0 {
		if e.o.Chain[len(e.o.Chain)-1] != e.o.Module {
			return nil, fmt.Errorf("цепочка должна кончаться модулем «%s»", e.o.Module)
		}
		return e.o.Chain, g.Check(e.o.Chain)
	}
	cs, err := g.Chains(e.o.Module)
	if err != nil {
		return nil, err
	}
	switch len(cs) {
	case 0:
		return nil, fmt.Errorf("от зерна до «%s» нет пути в графе", e.o.Module)
	case 1:
		return cs[0], nil
	}
	var b strings.Builder
	for i, c := range cs {
		if i == 8 {
			fmt.Fprintf(&b, "\n  … ещё %d", len(cs)-8)
			break
		}
		fmt.Fprintf(&b, "\n  --chain %s", strings.Join(c, ","))
	}
	return nil, fmt.Errorf("до «%s» несколько цепочек, задай одну:%s", e.o.Module, b.String())
}

// Run выполняет land по параметрам.
func Run(o Options) (Result, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	e := &env{o: o, res: &Result{}}
	if err := e.validate(); err != nil {
		return *e.res, err
	}
	cur, err := e.current()
	if err != nil {
		return *e.res, err
	}
	isModule := branchRe.MatchString(cur)
	switch {
	case cur == Dev:
		return *e.res, fmt.Errorf("на %s нечего вливать: перейди на ветку фичи или модуля", Dev)
	case isModule:
		if o.Module != "" || len(o.Chain) > 0 || o.Seed {
			return *e.res, fmt.Errorf("на ветке модуля флаги --module, --chain и --seed не нужны: ветка сливается в свой родитель")
		}
		err = e.mergeUp(cur)
	case o.Seed:
		if o.Module != "" || len(o.Chain) > 0 {
			return *e.res, fmt.Errorf("--seed несовместим с --module и --chain")
		}
		err = e.seed(cur)
	case o.Module != "":
		err = e.feature(cur)
	default:
		err = fmt.Errorf("на ветке фичи нужен --module M или --seed")
	}
	e.res.Branch = e.try("symbolic-ref", "-q", "--short", "HEAD")
	return *e.res, err
}

func (e *env) ahead(target, src string) error {
	if e.exists(target) && e.count0(target, src) == 0 {
		return fmt.Errorf("в %s нет коммитов, которых нет в %s", src, target)
	}
	return nil
}

func (e *env) count0(a, b string) int {
	n, _ := strconv.Atoi(e.try("rev-list", "--count", a+".."+b))
	return n
}

// squash вливает src в target одним коммитом; при неудаче возвращает всё как было.
func (e *env) squash(target, src, msg, back string) error {
	return gitx.SquashInto(e.o.Root, target, src, msg, back)
}

func (e *env) seed(cur string) error {
	if err := e.ahead(Dev, cur); err != nil {
		return err
	}
	tip, _, err := e.lastNamed(Dev)
	if err != nil {
		return err
	}
	v := named.Version{Full: true, R: tip.R, B: tip.B, C: tip.C + 1, T: tip.T + 1, Hash: e.short(cur)}
	msg := named.Message(e.o.Summary, v)
	e.say("зерно: %s → %s (squash)", cur, Dev)
	e.say("  %s", msg)
	e.res.Message = msg
	if e.o.Dry {
		return nil
	}
	return e.squash(Dev, cur, msg, cur)
}

func (e *env) feature(cur string) error {
	g, err := e.loadGraph()
	if err != nil {
		return err
	}
	chain, err := e.resolveChain(g)
	if err != nil {
		return err
	}
	steps, err := e.plan(chain)
	if err != nil {
		return err
	}
	last := steps[len(steps)-1]
	if last.create && e.count0(Dev, cur) == 0 {
		return fmt.Errorf("в %s нет коммитов, которых нет в %s", cur, Dev)
	}
	tip, _, err := e.lastNamed(Dev)
	if err != nil {
		return err
	}
	baseC, inBranch := tip.C, 0
	if !last.create {
		mb := e.try("merge-base", last.name, Dev)
		b, _, err := e.lastNamed(mb)
		if err != nil {
			return err
		}
		baseC, inBranch = b.C, e.count(Dev, last.name)
		if err := e.ahead(last.name, cur); err != nil {
			return err
		}
	}
	v := named.Version{C: baseC + inBranch + 1, Hash: e.short(cur)}
	msg := named.Message(e.o.Summary, v)
	e.say("цепочка: %s → %s", Dev, strings.Join(chain, " → "))
	for _, s := range steps {
		if s.create {
			e.say("  ветка %s от %s (создаётся)", s.name, s.parent)
		} else {
			e.say("  ветка %s (открытая)", s.name)
		}
	}
	e.say("фича: %s → %s (squash)", cur, last.name)
	e.say("  %s", msg)
	e.res.Message = msg
	if e.o.Dry {
		return nil
	}
	for _, s := range steps {
		if !s.create {
			continue
		}
		if _, err := e.git("branch", s.name, s.parent); err != nil {
			return err
		}
		if err := e.setCfg(s.name, "sky-parent", s.parent); err != nil {
			return err
		}
		if err := e.setCfg(s.name, "sky-state", "open"); err != nil {
			return err
		}
	}
	return e.squash(last.name, cur, msg, cur)
}

func (e *env) openChildren(branch string) []string {
	var kids []string
	for _, b := range strings.Split(e.try("for-each-ref", "--format=%(refname:short)", "refs/heads/"), "\n") {
		if branchRe.MatchString(b) && e.cfg(b, "sky-parent") == branch && e.cfg(b, "sky-state") == "open" {
			kids = append(kids, b)
		}
	}
	return kids
}

func (e *env) mergeUp(cur string) error {
	if e.cfg(cur, "sky-state") != "open" {
		return fmt.Errorf("ветка %s не открыта (уже влита или создана не sky land)", cur)
	}
	parent := e.cfg(cur, "sky-parent")
	if parent == "" || !e.exists(parent) {
		return fmt.Errorf("у ветки %s нет родителя (sky-parent): слить её нельзя", cur)
	}
	if kids := e.openChildren(cur); len(kids) > 0 {
		return fmt.Errorf("сначала влей вложенные ветки (снизу вверх): %s", strings.Join(kids, ", "))
	}
	if e.count0(parent, cur) == 0 {
		return fmt.Errorf("в %s нет коммитов, которых нет в %s", cur, parent)
	}
	var v named.Version
	n := e.count(parent, cur)
	if parent == Dev {
		tip, _, err := e.lastNamed(Dev)
		if err != nil {
			return err
		}
		c := tip.C + n + 1
		v = named.Version{Full: true, R: tip.R, B: tip.B, C: c, T: tip.T + (c - tip.C)}
	} else {
		mb := e.try("merge-base", parent, Dev)
		b, _, err := e.lastNamed(mb)
		if err != nil {
			return err
		}
		v = named.Version{C: b.C + e.count(Dev, parent) + n + 1}
	}
	v.Hash = e.short(cur)
	msg := named.Message(e.o.Summary, v)
	e.say("слияние вверх: %s → %s (merge-коммит)", cur, parent)
	e.say("  %s", msg)
	e.res.Message = msg
	if e.o.Dry {
		return nil
	}
	if err := e.checkout(parent); err != nil {
		return err
	}
	if _, err := e.git("merge", "--no-ff", "-q", "-m", msg, cur); err != nil {
		e.git("merge", "--abort")
		e.checkout(cur)
		return fmt.Errorf("слияние %s → %s не удалось, ничего не изменено: %v", cur, parent, err)
	}
	return e.setCfg(cur, "sky-state", "merged")
}
