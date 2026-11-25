package merge

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/pr"
)

type repo struct {
	t   *testing.T
	dir string
}

func (r repo) git(args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.dir}, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r repo) commit(name, msg string) {
	r.t.Helper()
	p := filepath.Join(r.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(msg+"\n"), 0o644)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func clone(t *testing.T, bare, name string) repo {
	t.Helper()
	dir := filepath.Join(filepath.Dir(bare), name)
	if out, err := exec.Command("git", "clone", "-q", bare, dir).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	r := repo{t: t, dir: dir}
	r.git("config", "user.name", name)
	r.git("config", "user.email", name+"@t")
	r.git("config", "commit.gpgsign", "false")
	return r
}

const graphDoc = "| Модуль | Зависит от |\n|---|---|\n| leaf | — |\n| main | leaf |\n"

type world struct {
	t      *testing.T
	me     repo // тот, кто ревьюит и запускает merge
	author repo
	bare   string
	ghLog  string
	view   string
}

// newWorld — origin с dev (граф и именованный коммит v1.1.1+1), me и author — клоны; подставной gh в PATH.
func newWorld(t *testing.T) *world {
	t.Helper()
	base := t.TempDir()
	w := &world{t: t, bare: filepath.Join(base, "origin.git"), ghLog: filepath.Join(base, "gh.log"), view: filepath.Join(base, "view.json")}
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "dev", w.bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	w.author = clone(t, w.bare, "author")
	w.author.git("checkout", "-q", "-b", "dev")
	w.author.commit("docs/PROJECT-GRAPH.md", "chore: graph")
	os.WriteFile(filepath.Join(w.author.dir, "docs", "PROJECT-GRAPH.md"), []byte(graphDoc), 0o644)
	w.author.git("add", "-A")
	w.author.git("commit", "-q", "-m", "Add graph | v1.1.1+1.abc1234")
	w.author.git("push", "-q", "origin", "dev")
	w.me = clone(t, w.bare, "me")
	w.me.git("checkout", "-q", "dev")
	gh := "#!/bin/sh\necho \"$@\" >> \"" + w.ghLog + "\"\ncase \"$1 $2\" in\n\"pr view\") cat \"" + w.view + "\";;\nesac\n"
	bin := filepath.Join(base, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return w
}

// openPR — автор делает ветку от from, коммит и пушит; gh отвечает, что PR #7 открыт с этим описанием.
func (w *world) openPR(from, branch string, o pr.Options) {
	w.t.Helper()
	w.author.git("fetch", "-q", "origin")
	w.author.git("checkout", "-q", "-B", branch, "origin/"+from)
	w.author.commit("feat-"+filepath.Base(branch)+".txt", "feat: "+branch)
	w.author.git("push", "-q", "origin", branch)
	o.Branch, o.Summary = branch, "Add "+filepath.Base(branch)
	b, _ := json.Marshal(map[string]any{"number": 7, "state": "OPEN", "headRefName": branch, "baseRefName": from, "title": o.Summary, "body": pr.Body(o), "url": "https://github.com/o/r/pull/7"})
	os.WriteFile(w.view, b, 0o644)
}

func (w *world) opts() Options { return Options{Root: w.me.dir, PR: "7"} }

func (w *world) remote(branch string) string {
	out := w.me.git("ls-remote", "--heads", "origin", branch)
	f := strings.Fields(out)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func TestMergeSeedPR(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/x", pr.Options{Kind: "seed"})
	res, err := Run(w.opts())
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Lines, "\n"))
	}
	subj := w.me.git("log", "-1", "--format=%s", "dev")
	if !strings.HasPrefix(subj, "Add x | v1.1.2+2.") {
		t.Errorf("сообщение: %q", subj)
	}
	if w.remote("dev") != w.me.git("rev-parse", "dev") {
		t.Error("dev должна уйти в origin")
	}
	log, _ := os.ReadFile(w.ghLog)
	if !strings.Contains(string(log), "pr close 7 --delete-branch --comment Влито sky merge: Add x | v1.1.2+2.") {
		t.Errorf("закрытие PR: %s", log)
	}
	if w.me.git("branch", "--list", "tmp-merge-*") != "" {
		t.Error("временная ветка должна исчезнуть")
	}
	if w.me.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("HEAD вернулся на исходную ветку")
	}
}

func TestMergeModulePRThenUp(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/m", pr.Options{Kind: "module", Module: "leaf", Chain: []string{"leaf"}})
	if _, err := Run(w.opts()); err != nil {
		t.Fatal(err)
	}
	branches := w.me.git("branch", "--list", "leaf/*", "--format=%(refname:short)")
	if branches == "" || w.remote(branches) == "" {
		t.Fatalf("ветка модуля должна быть создана и запушена: %q", branches)
	}
	if w.remote("dev") != w.me.git("rev-parse", "dev") || strings.Contains(w.me.git("log", "-1", "--format=%s", "dev"), "Add m") {
		t.Error("squash идёт в ветку модуля, dev не меняется")
	}
	if !strings.HasPrefix(w.me.git("log", "-1", "--format=%s", branches), "Add m | v2.") {
		t.Errorf("сообщение на ветке модуля: %q", w.me.git("log", "-1", "--format=%s", branches))
	}
	w.me.git("checkout", "-q", branches)
	res, err := Run(Options{Root: w.me.dir, Up: true, Summary: "Add leaf work"})
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Lines, "\n"))
	}
	if !strings.HasPrefix(w.me.git("log", "-1", "--format=%s", "dev"), "Add leaf work | v1.1.3+3.") || w.remote("dev") != w.me.git("rev-parse", "dev") {
		t.Errorf("merge вверх: %q", w.me.git("log", "-1", "--format=%s", "dev"))
	}
	if w.me.git("symbolic-ref", "--short", "HEAD") != branches {
		t.Error("после --up HEAD остаётся на исходной ветке")
	}
}

func TestMergeFixPR(t *testing.T) {
	w := newWorld(t)
	tip, tree := w.me.git("rev-parse", "dev"), w.me.git("rev-parse", "dev^{tree}")
	short := w.me.git("rev-parse", "--short=7", "dev")
	tail := w.me.git("commit-tree", tree, "-p", tip, "-m", "Build 1 alpha | v1.a1.1+1."+short)
	w.me.git("branch", "build/1125-1020", tail)
	w.me.git("push", "-q", "origin", "build/1125-1020")
	w.openPR("build/1125-1020", "wip/1125-1030/hot", pr.Options{Kind: "fix", Base: "build/1125-1020"})
	if _, err := Run(w.opts()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w.me.git("log", "-1", "--format=%s", "build/1125-1020"), "Add hot | v1.a1-1.1+2.") {
		t.Errorf("%q", w.me.git("log", "-1", "--format=%s", "build/1125-1020"))
	}
	if w.remote("build/1125-1020") != w.me.git("rev-parse", "build/1125-1020") {
		t.Error("сборка должна уйти в origin")
	}
}

func TestMergeDryChangesNothing(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/x", pr.Options{Kind: "seed"})
	before, rem := w.me.git("rev-parse", "dev"), w.remote("dev")
	o := w.opts()
	o.Dry = true
	res, err := Run(o)
	if err != nil || w.me.git("rev-parse", "dev") != before || w.remote("dev") != rem || len(res.Pushed) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	if w.me.git("branch", "--list", "tmp-merge-*") != "" || w.me.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("dry оставил следы")
	}
	if log, _ := os.ReadFile(w.ghLog); strings.Contains(string(log), "pr close") {
		t.Error("dry не закрывает PR")
	}
}

func TestMergeRefusals(t *testing.T) {
	w := newWorld(t)
	if _, err := Run(Options{Root: w.me.dir}); err == nil {
		t.Fatal("нужен PR или --up")
	}
	if _, err := Run(Options{Root: w.me.dir, PR: "7", Up: true}); err == nil {
		t.Fatal("PR и --up вместе нельзя")
	}
	// PR открыт не через sky pr
	b, _ := json.Marshal(map[string]any{"number": 7, "state": "OPEN", "headRefName": "x", "title": "T", "body": "no block"})
	os.WriteFile(w.view, b, 0o644)
	if _, err := Run(w.opts()); err == nil || !strings.Contains(err.Error(), "sky-target") {
		t.Fatalf("%v", err)
	}
	// PR закрыт
	b, _ = json.Marshal(map[string]any{"number": 7, "state": "MERGED", "headRefName": "x", "title": "T", "body": pr.Body(pr.Options{Kind: "seed"})})
	os.WriteFile(w.view, b, 0o644)
	if _, err := Run(w.opts()); err == nil || !strings.Contains(err.Error(), "не открыт") {
		t.Fatalf("%v", err)
	}
	// разошедшаяся dev не сливается
	w.author.git("checkout", "-q", "dev")
	w.author.commit("o.txt", "feat: theirs")
	w.author.git("push", "-q", "origin", "dev")
	w.me.commit("m.txt", "feat: mine")
	w.openPR("dev", "wip/1125-1005/y", pr.Options{Kind: "seed"})
	if _, err := Run(w.opts()); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("%v", err)
	}
}

func TestMergeStaleTmpCleaned(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/s", pr.Options{Kind: "seed"})
	w.me.git("branch", "tmp-merge-7", "dev")
	res, err := Run(w.opts())
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Lines, "\n"))
	}
	if w.me.git("branch", "--list", "tmp-merge-*") != "" {
		t.Error("временная ветка прошлой попытки должна быть убрана")
	}
}

func TestMergePushFailureRollsBack(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/r", pr.Options{Kind: "seed"})
	hook := filepath.Join(w.bare, "hooks", "pre-receive")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	before := w.me.git("rev-parse", "dev")
	_, err := Run(w.opts())
	if err == nil || !strings.Contains(err.Error(), "возвращено как было") {
		t.Fatalf("ожидался откат, err=%v", err)
	}
	if w.me.git("rev-parse", "dev") != before {
		t.Error("dev должна вернуться")
	}
	if w.me.git("branch", "--list", "tmp-merge-*") != "" {
		t.Error("временная ветка должна исчезнуть")
	}
	os.Remove(hook)
	if _, err := Run(w.opts()); err != nil {
		t.Fatalf("повтор после отката: %v", err)
	}
	if w.remote("dev") != w.me.git("rev-parse", "dev") {
		t.Error("повтор должен дойти до origin")
	}
}

func TestMergeAllClimbsToDev(t *testing.T) {
	w := newWorld(t)
	w.openPR("dev", "wip/1125-1005/a", pr.Options{Kind: "module", Module: "leaf", Chain: []string{"leaf"}})
	o := w.opts()
	o.All = true
	res, err := Run(o)
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Lines, "\n"))
	}
	if !strings.HasPrefix(w.me.git("log", "-1", "--format=%s", "dev"), "Add a | v1.1.") || w.remote("dev") != w.me.git("rev-parse", "dev") {
		t.Errorf("dev после --all: %q", w.me.git("log", "-1", "--format=%s", "dev"))
	}
	if w.me.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("HEAD вернулся")
	}
}
