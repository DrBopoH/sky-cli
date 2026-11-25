// Package merge — удалённая часть, этап R3: слияние в общую область. Это голова поверх land и fix (теория, 4.2):
// версия, цепочка и тип слияния считаются теми же алгоритмами, что и локально, а здесь добавлен только перенос:
// подтянуть общие ветки, взять ветку PR из origin, прогнать её через land или fix, запушить то, что изменилось
// в общих ветках, и закрыть PR. Запускает слияние тот, кто ревьюит (регламент, 3.1.3): автор свои ветки не пушит.
package merge

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/fix"
	"github.com/DrBopoH/sky-cli/internal/ghx"
	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/land"
	"github.com/DrBopoH/sky-cli/internal/modules"
	"github.com/DrBopoH/sky-cli/internal/pr"
	"github.com/DrBopoH/sky-cli/internal/sync"
)

// Dev — ветка разработки, куда доходит --all.
const Dev = "dev"

// Options — параметры одного вызова.
type Options struct {
	Root    string
	PR      string // номер или адрес PR; пусто с Up
	Up      bool   // слить ветку модуля, на которой стоит HEAD, в её родителя (без PR)
	All     bool   // с PR на модуль: после squash довести слияние вверх по цепочке до dev
	Summary string // для Up: резюме именованного коммита слияния
	Dry     bool
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines   []string
	Pushed  []string
	Message string
}

type prInfo struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	Head    string `json:"headRefName"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	URL     string `json:"url"`
	BaseRef string `json:"baseRefName"`
}

type env struct {
	o    Options
	res  *Result
	back string // ветка, на которой стоял HEAD
}

func (e *env) say(f string, a ...any) { e.res.Lines = append(e.res.Lines, fmt.Sprintf(f, a...)) }

// Run выполняет R3 по параметрам.
func Run(o Options) (Result, error) {
	e := &env{o: o, res: &Result{}}
	err := e.run()
	return *e.res, err
}

func (e *env) run() error {
	root := e.o.Root
	if e.o.Up == (e.o.PR != "") {
		return fmt.Errorf("нужно что-то одно: номер PR или --up \"резюме\"")
	}
	if !gitx.OK(root, "remote", "get-url", sync.Remote) {
		return fmt.Errorf("нет удалённого репозитория %s", sync.Remote)
	}
	if d := gitx.Dirty(root); d != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: закоммить (sky commit) или убери\n%s", d)
	}
	e.back = strings.TrimSpace(gitx.Try(root, "symbolic-ref", "-q", "--short", "HEAD"))
	if e.o.Up {
		return e.up()
	}
	return e.pull()
}

// shared — вершины общих веток: по ним видно, что слияние изменило.
func (e *env) shared() map[string]string {
	out := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSpace(gitx.Try(e.o.Root, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/")), "\n") {
		n, h, ok := strings.Cut(ln, " ")
		if ok && sync.Shared(n) {
			out[n] = h
		}
	}
	return out
}

// sync подтягивает общие ветки; расхождение истории или грязная ветка делают слияние небезопасным.
func (e *env) sync() error {
	res, err := sync.Run(sync.Options{Root: e.o.Root})
	if err != nil {
		return err
	}
	for _, it := range res.Items {
		switch it.State {
		case sync.Diverged, sync.Blocked:
			return fmt.Errorf("ветка %s не совпадает с %s/%s (%s): сначала реши это вручную (sky sync покажет)", it.Name, sync.Remote, it.Name, it.State)
		case sync.Ahead:
			e.say("⚠ %s впереди %s на %d коммитов: если слияние изменит эту ветку, они уйдут вместе с ним", it.Name, sync.Remote, it.N)
		}
	}
	return nil
}

// snap — состояние общих веток до слияния: вершины и sky-state, чтобы вернуть всё как было.
type snap struct {
	refs   map[string]string
	states map[string]string
}

func (e *env) snapshot() snap {
	s := snap{refs: e.shared(), states: map[string]string{}}
	for n := range s.refs {
		s.states[n] = gitx.BranchConfig(e.o.Root, n, "sky-state")
	}
	return s
}

// push отправляет общие ветки, которые изменились или появились. Если ни одна ветка не ушла, а слияние не удалось,
// локальное состояние возвращается как было (повторить можно сразу); если часть уже ушла, откат был бы хуже.
func (e *env) push(before snap) error {
	after := e.shared()
	var names []string
	for n, h := range after {
		if before.refs[n] != h {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if _, err := gitx.Run(e.o.Root, "push", "-u", sync.Remote, n); err != nil {
			if len(e.res.Pushed) == 0 {
				e.rollback(before)
				return fmt.Errorf("push ветки %s не удался, всё возвращено как было (повтори команду): %v", n, err)
			}
			return fmt.Errorf("ветка %s изменена локально, но push не удался (ушло: %s; повтори git push %s %s): %v", n, strings.Join(e.res.Pushed, ", "), sync.Remote, n, err)
		}
		e.res.Pushed = append(e.res.Pushed, n)
		e.say("  push %s %s", sync.Remote, n)
	}
	return nil
}

// rollback возвращает общие ветки к снимку: новые удаляет, сдвинутые возвращает, sky-state восстанавливает.
func (e *env) rollback(before snap) {
	root := e.o.Root
	e.restore()
	for n, h := range e.shared() {
		old, existed := before.refs[n]
		switch {
		case !existed:
			gitx.Run(root, "branch", "-D", n)
		case old != h && n == e.back:
			gitx.Run(root, "reset", "-q", "--hard", old)
		case old != h:
			gitx.Run(root, "update-ref", "-m", "sky merge rollback", "refs/heads/"+n, old, h)
		}
	}
	for n, st := range before.states {
		if _, still := e.shared()[n]; still && st != "" && gitx.BranchConfig(root, n, "sky-state") != st {
			gitx.Run(root, "config", "branch."+n+".sky-state", st)
		}
	}
}

func (e *env) restore() {
	if e.back != "" {
		gitx.Run(e.o.Root, "checkout", "-q", e.back)
	}
}

func (e *env) up() error {
	if e.back == "" {
		return fmt.Errorf("HEAD не на ветке: перейди на ветку модуля")
	}
	if e.o.Dry {
		r, err := land.Run(land.Options{Root: e.o.Root, Summary: e.o.Summary, Dry: true})
		e.res.Lines = append(e.res.Lines, r.Lines...)
		e.res.Message = r.Message
		return err
	}
	if err := e.sync(); err != nil {
		return err
	}
	before := e.snapshot()
	r, err := land.Run(land.Options{Root: e.o.Root, Summary: e.o.Summary})
	e.res.Lines = append(e.res.Lines, r.Lines...)
	e.res.Message = r.Message
	if err != nil {
		e.rollback(before)
		return err
	}
	if err := e.push(before); err != nil {
		return err
	}
	e.restoreIfSwitched(r.Branch)
	e.say("✓ %s", r.Message)
	e.hintUp(r.Branch)
	return nil
}

// hintUp подсказывает следующий шаг, если ветка модуля, куда всё легло, ещё не слита до dev.
func (e *env) hintUp(branch string) {
	if _, _, ok := modules.ParseBranch(branch); ok {
		e.say("  дальше: ветка %s ждёт слияния вверх (на ней sky merge --up \"резюме\", либо в следующий раз sky merge <PR> --all)", branch)
	}
}

// restoreIfSwitched возвращает HEAD на исходную ветку, если она ещё есть.
func (e *env) restoreIfSwitched(now string) {
	if now != e.back && gitx.OK(e.o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+e.back) {
		e.restore()
	}
}

func (e *env) pull() error {
	root := e.o.Root
	if !ghx.Available() {
		return fmt.Errorf("не найдена команда gh (GitHub CLI): установи её и войди (gh auth login)")
	}
	out, err := ghx.Run(root, "pr", "view", e.o.PR, "--json", "number,state,headRefName,baseRefName,title,body,url")
	if err != nil {
		return err
	}
	var p prInfo
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		return fmt.Errorf("ответ gh не разобран: %v", err)
	}
	if p.State != "OPEN" {
		return fmt.Errorf("PR #%d не открыт (%s)", p.Number, p.State)
	}
	t, ok := pr.ParseBody(p.Body)
	if !ok {
		return fmt.Errorf("в описании PR #%d нет блока sky-target: он открыт не через sky pr, цель слияния неизвестна", p.Number)
	}
	e.say("PR #%d: %s (%s → цель %s)", p.Number, p.Title, p.Head, describe(t))
	if !e.o.Dry {
		if err := e.sync(); err != nil {
			return err
		}
	}
	tmp := fmt.Sprintf("tmp-merge-%d", p.Number)
	ref := sync.Remote + "/" + p.Head
	// fetch одной ветки PR меняет только ссылки origin/…, поэтому нужен и в --dry
	if _, err := gitx.Run(root, "fetch", "-q", sync.Remote, p.Head); err != nil {
		return fmt.Errorf("ветка %s не получена из %s: %v", p.Head, sync.Remote, err)
	}
	if !gitx.OK(root, "rev-parse", "--verify", "-q", "refs/remotes/"+ref) {
		return fmt.Errorf("нет ветки %s в %s: PR ссылается на неё", p.Head, sync.Remote)
	}
	if gitx.OK(root, "rev-parse", "--verify", "-q", "refs/heads/"+tmp) {
		if e.back == tmp {
			return fmt.Errorf("HEAD на временной ветке %s: перейди на другую ветку и повтори", tmp)
		}
		// служебная ветка прерванной попытки: пересоздаётся заново от origin
		gitx.Run(root, "branch", "-D", tmp)
		e.say("· временная ветка %s от прерванной попытки удалена", tmp)
	}
	if _, err := gitx.Run(root, "checkout", "-q", "-b", tmp, ref); err != nil {
		return err
	}
	defer func() {
		e.restore()
		gitx.Run(root, "branch", "-D", tmp)
	}()
	before := e.snapshot()
	var msg, landed string
	if t.Kind == "fix" {
		r, err := fix.Run(fix.Options{Root: root, Summary: p.Title, Build: t.Build, Dry: e.o.Dry})
		e.res.Lines = append(e.res.Lines, r.Lines...)
		if err != nil {
			e.rollback(before)
			return err
		}
		msg, landed = r.Message, r.Branch
	} else {
		r, err := land.Run(land.Options{Root: root, Summary: p.Title, Module: t.Module, Chain: t.Chain, Seed: t.Kind == "seed", Dry: e.o.Dry})
		e.res.Lines = append(e.res.Lines, r.Lines...)
		if err != nil {
			e.rollback(before)
			return err
		}
		msg, landed = r.Message, r.Branch
	}
	e.res.Message = msg
	if e.o.Dry {
		return nil
	}
	if e.o.All {
		if msg, err = e.climb(p.Title); err != nil {
			e.rollback(before)
			return err
		}
		landed = Dev
		e.res.Message = msg
	}
	if err := e.push(before); err != nil {
		return err
	}
	comment := fmt.Sprintf("Влито sky merge: %s", msg)
	if _, err := ghx.Run(root, "pr", "close", fmt.Sprint(p.Number), "--delete-branch", "--comment", comment); err != nil {
		return fmt.Errorf("слияние выполнено и запушено, но PR не закрыт: %v", err)
	}
	e.say("✓ PR #%d закрыт: %s", p.Number, msg)
	e.hintUp(landed)
	return nil
}

// climb доводит слияние от ветки модуля, на которой стоит HEAD, вверх по цепочке до dev. Возвращает сообщение последнего слияния.
func (e *env) climb(summary string) (string, error) {
	var msg string
	for {
		cur := strings.TrimSpace(gitx.Try(e.o.Root, "symbolic-ref", "-q", "--short", "HEAD"))
		if _, _, ok := modules.ParseBranch(cur); !ok {
			if msg == "" {
				return "", fmt.Errorf("--all нужен для PR на модуль: слитое лежит не в ветке модуля (%s)", cur)
			}
			return msg, nil
		}
		r, err := land.Run(land.Options{Root: e.o.Root, Summary: summary})
		e.res.Lines = append(e.res.Lines, r.Lines...)
		if err != nil {
			return "", err
		}
		msg = r.Message
	}
}

func describe(t pr.Target) string {
	switch t.Kind {
	case "module":
		return "модуль " + t.Module + " (dev → " + strings.Join(t.Chain, " → ") + ")"
	case "fix":
		return "исправление в " + t.Build
	}
	return "зерно (dev)"
}
