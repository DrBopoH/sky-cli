// Package build — сборка по регламенту (3.6): запускает команды сборки из sky.conf, ждёт успеха и только тогда
// создаёт ветку сборки «build/ММДД-ЧЧММ» с хвостом — коммитом с тем же деревом, что у вершины dev.
// Ветка сборки в dev не вливается; HEAD остаётся на dev.
package build

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/named"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Dev     = "dev"
	Prefix  = "build/" // ветки сборок: build/ММДД-ЧЧММ
	MaxLen  = 72
	tailMax = 4000
)

// Options — параметры одного вызова.
type Options struct {
	Root   string
	From   string // «R.B.C+T»: точка отсчёта, пока в dev нет именованных коммитов
	Points []conf.BuildPoint
	State  byte   // a, b, s, m; 0 — a
	Title  string // название сборки; пусто — «Build N alpha»
	Dry    bool
	Now    time.Time
	Out    io.Writer // куда стримить вывод команд; nil — никуда
}

// Run — итог запуска одной точки сборки.
type Run struct {
	Point    conf.BuildPoint
	Code     int
	Duration time.Duration
	Tail     string // хвост вывода
}

// Result — что сделано.
type Result struct {
	Lines    []string
	Message  string
	Branch   string
	Runs     []Run
	Skipped  bool   // в sky.conf нет build: запуск пропущен
	Done     bool   // ветка создана
	Markdown string // отчёт для foryou/build.md
}

var stateName = map[byte]string{'a': "alpha", 'b': "beta", 's': "stable", 'm': "m"}

type env struct {
	o   Options
	res *Result
	h   vhist.H
}

func (e *env) say(f string, a ...any) { e.res.Lines = append(e.res.Lines, fmt.Sprintf(f, a...)) }

// Title собирает название: «Build 2 stable» или «Build 2 stable: свой текст».
func Title(b int, state byte, custom string) string {
	t := fmt.Sprintf("Build %d %s", b, stateName[state])
	if strings.TrimSpace(custom) != "" {
		t += ": " + strings.TrimSpace(custom)
	}
	return t
}

// Exec выполняет сборку по параметрам.
func Exec(o Options) (Result, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.State == 0 {
		o.State = 'a'
	}
	e := &env{o: o, res: &Result{}, h: vhist.H{Root: o.Root, From: o.From}}
	err := e.run()
	e.res.Markdown = e.markdown(err)
	return *e.res, err
}

func (e *env) run() error {
	o := e.o
	if _, ok := stateName[o.State]; !ok {
		return fmt.Errorf("состояние сборки: a, b, s или m")
	}
	if !gitx.OK(o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+Dev) {
		return fmt.Errorf("нет ветки %s", Dev)
	}
	if cur := strings.TrimSpace(gitx.Try(o.Root, "symbolic-ref", "-q", "--short", "HEAD")); cur != Dev {
		return fmt.Errorf("сборка снимается с %s: перейди на неё (сейчас: %s)", Dev, orNone(cur))
	}
	if d := gitx.Dirty(o.Root); d != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: сборка снимается с коммита dev, закоммить (sky commit) или убери\n%s", d)
	}
	tip, found, err := e.h.Last(Dev)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("в %s нет именованных коммитов: сборке не с чего сниматься (сначала sky land)", Dev)
	}
	if b, ok := e.h.BuildOf(tip.R, tip.B); ok {
		return fmt.Errorf("сборка %d.%d уже снята (%s): следующая — после нового именованного коммита в %s", tip.R, tip.B, orNone(e.h.Branch(b)), Dev)
	}
	title := Title(tip.B, o.State, o.Title)
	if utf8.RuneCountInString(title) > MaxLen || strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("название сборки одной строкой не длиннее %d символов", MaxLen)
	}
	tipHash := strings.TrimSpace(gitx.Try(o.Root, "rev-parse", "--verify", Dev))
	v := named.Version{Full: true, R: tip.R, B: tip.B, C: tip.C, T: tip.T, State: o.State, Hash: strings.TrimSpace(gitx.Try(o.Root, "rev-parse", "--short=7", tipHash))}
	msg := named.Message(title, v)
	branch := Prefix + o.Now.Format("0102-1504")
	if gitx.OK(o.Root, "rev-parse", "--verify", "-q", "refs/heads/"+branch) {
		return fmt.Errorf("ветка %s уже есть: подожди минуту", branch)
	}
	e.res.Message, e.res.Branch = msg, branch
	for _, p := range o.Points {
		if st, err := os.Stat(e.dir(p)); err != nil || !st.IsDir() {
			return fmt.Errorf("точка сборки «%s %s»: нет папки %s", p.Dir, p.Cmd, p.Dir)
		}
	}
	e.say("сборка %d.%d от вершины %s (%s)", tip.R, tip.B, Dev, v.Hash)
	if len(o.Points) == 0 {
		e.res.Skipped = true
		e.say("в sky.conf нет строк build: запуск сборки пропущен, компиляция не выполнялась")
		e.say("  добавь «build = <папка> <команда>» (например: build = backend/ go build ./...)")
	}
	for _, p := range o.Points {
		e.say("  точка: %s → %s", p.Dir, p.Cmd)
	}
	e.say("  ветка %s: хвост — копия дерева %s", branch, Dev)
	e.say("  %s", msg)
	if o.Dry {
		return nil
	}
	for _, p := range o.Points {
		r := e.exec(p)
		e.res.Runs = append(e.res.Runs, r)
		if r.Code != 0 {
			return fmt.Errorf("сборка не удалась на «%s %s» (код %d): ветка не создана", p.Dir, p.Cmd, r.Code)
		}
		e.say("✓ %s %s (%s)", p.Dir, p.Cmd, r.Duration.Round(time.Millisecond))
	}
	tree := strings.TrimSpace(gitx.Try(o.Root, "rev-parse", tipHash+"^{tree}"))
	commit, err := gitx.Run(o.Root, "commit-tree", tree, "-p", tipHash, "-m", msg)
	if err != nil {
		return err
	}
	if _, err := gitx.Run(o.Root, "branch", branch, commit); err != nil {
		return err
	}
	e.res.Done = true
	return nil
}

func (e *env) dir(p conf.BuildPoint) string {
	if p.Dir == "./" {
		return e.o.Root
	}
	return filepath.Join(e.o.Root, filepath.FromSlash(p.Dir))
}

func (e *env) exec(p conf.BuildPoint) Run {
	var tail bytes.Buffer
	var w io.Writer = &tail
	if e.o.Out != nil {
		w = io.MultiWriter(e.o.Out, &tail)
	}
	if e.o.Out != nil {
		fmt.Fprintf(e.o.Out, "$ [%s] %s\n", p.Dir, p.Cmd)
	}
	c := exec.Command("sh", "-c", p.Cmd)
	c.Dir = e.dir(p)
	c.Stdout, c.Stderr = w, w
	start := time.Now()
	err := c.Run()
	r := Run{Point: p, Duration: time.Since(start)}
	if err != nil {
		r.Code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			r.Code = ee.ExitCode()
		} else {
			fmt.Fprintf(&tail, "%v\n", err)
		}
	}
	t := tail.String()
	if len(t) > tailMax {
		t = "…" + t[len(t)-tailMax:]
	}
	r.Tail = strings.TrimSpace(t)
	return r
}

func (e *env) markdown(err error) string {
	var b strings.Builder
	b.WriteString("# Последняя сборка\n\n")
	b.WriteString("Файл пишет `sky build`; следующая сборка его заменяет.\n\n")
	switch {
	case err != nil && !e.res.Done:
		fmt.Fprintf(&b, "Итог: **не создана** — %s\n", strings.SplitN(err.Error(), "\n", 2)[0])
	case e.o.Dry:
		b.WriteString("Итог: пробный запуск (`--dry`), ничего не менялось\n")
	default:
		fmt.Fprintf(&b, "Итог: **создана** ветка `%s`\n", e.res.Branch)
	}
	if e.res.Message != "" {
		fmt.Fprintf(&b, "\nСообщение хвоста: `%s`\n", e.res.Message)
	}
	if e.res.Skipped {
		b.WriteString("\nВ sky.conf нет строк `build`: компиляция не выполнялась.\n")
	}
	if len(e.res.Runs) > 0 {
		b.WriteString("\n| Папка | Команда | Код | Время |\n|---|---|---|---|\n")
		for _, r := range e.res.Runs {
			fmt.Fprintf(&b, "| %s | `%s` | %d | %s |\n", r.Point.Dir, r.Point.Cmd, r.Code, r.Duration.Round(time.Millisecond))
		}
		last := e.res.Runs[len(e.res.Runs)-1]
		if last.Code != 0 && last.Tail != "" {
			fmt.Fprintf(&b, "\nКонец вывода упавшей команды:\n\n```\n%s\n```\n", last.Tail)
		}
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
