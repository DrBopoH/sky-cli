package build

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DrBopoH/sky-cli/internal/conf"
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

func (r repo) commit(name, body, msg string) {
	r.t.Helper()
	p := filepath.Join(r.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// newRepo — dev с именованными коммитами 1.1.28+28 и 1.1.29+29.
func newRepo(t *testing.T) repo {
	t.Helper()
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", Dev)
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.git("config", "commit.gpgsign", "false")
	r.commit("a.txt", "a\n", "Add x | v1.1.28+28.aaa1111")
	r.commit("b.txt", "b\n", "Add y | v1.1.29+29.abc1234")
	return r
}

var t0 = time.Date(2026, 11, 25, 10, 20, 0, 0, time.UTC)

func (r repo) build(o Options) (Result, error) {
	o.Root = r.dir
	if o.Now.IsZero() {
		o.Now = t0
	}
	return Exec(o)
}

func TestBuildCreatesBranchWithSameTree(t *testing.T) {
	r := newRepo(t)
	tip := r.git("rev-parse", "--short=7", "HEAD")
	var out bytes.Buffer
	res, err := r.build(Options{Points: []conf.BuildPoint{{Dir: "./", Cmd: "echo ok > built.txt"}}, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Done || res.Branch != "build/1125-1020" {
		t.Fatalf("%+v", res)
	}
	want := "Build 1 alpha | v1.a1.29+29." + tip
	if got := r.git("log", "-1", "--format=%s", res.Branch); got != want {
		t.Errorf("сообщение %q, нужно %q", got, want)
	}
	if r.git("rev-parse", res.Branch+"^{tree}") != r.git("rev-parse", "dev^{tree}") {
		t.Error("дерево хвоста не равно дереву dev")
	}
	if r.git("rev-parse", res.Branch+"^") != r.git("rev-parse", "dev") {
		t.Error("родитель хвоста — не вершина dev")
	}
	if r.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("HEAD ушёл с dev")
	}
	if !strings.Contains(out.String(), "$ [./] echo ok") {
		t.Errorf("вывод не стримится: %q", out.String())
	}
	if !strings.Contains(res.Markdown, "**создана**") {
		t.Error(res.Markdown)
	}
}

func TestBuildStateAndTitle(t *testing.T) {
	r := newRepo(t)
	res, err := r.build(Options{State: 's', Title: "macro and market data"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Message, "Build 1 stable: macro and market data | v1.s1.29+29.") {
		t.Error(res.Message)
	}
}

func TestBuildFailureCreatesNothingAndStopsFast(t *testing.T) {
	r := newRepo(t)
	pts := []conf.BuildPoint{
		{Dir: "./", Cmd: "echo boom; exit 3"},
		{Dir: "./", Cmd: "touch second.txt"},
	}
	res, err := r.build(Options{Points: pts})
	if err == nil || !strings.Contains(err.Error(), "код 3") {
		t.Fatalf("ожидалась ошибка кода 3: %v", err)
	}
	if res.Done || len(res.Runs) != 1 {
		t.Fatalf("%+v", res)
	}
	if r.git("branch", "--list", "build/*") != "" {
		t.Error("ветка создана после провала")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "second.txt")); err == nil {
		t.Error("вторая точка запущена после провала")
	}
	if !strings.Contains(res.Markdown, "boom") || !strings.Contains(res.Markdown, "**не создана**") {
		t.Error(res.Markdown)
	}
}

func TestBuildSeveralPointsInDirs(t *testing.T) {
	r := newRepo(t)
	os.MkdirAll(filepath.Join(r.dir, "backend"), 0o755)
	os.MkdirAll(filepath.Join(r.dir, "frontend"), 0o755)
	pts := []conf.BuildPoint{
		{Dir: "backend/", Cmd: "pwd > ../b.out"},
		{Dir: "frontend/", Cmd: "pwd > ../f.out"},
	}
	res, err := r.build(Options{Points: pts})
	if err != nil || len(res.Runs) != 2 {
		t.Fatalf("%v %+v", err, res)
	}
	b, _ := os.ReadFile(filepath.Join(r.dir, "b.out"))
	if !strings.HasSuffix(strings.TrimSpace(string(b)), "backend") {
		t.Errorf("команда выполнена не в папке точки: %q", b)
	}
}

func TestBuildNoPointsSkipsWithExplanation(t *testing.T) {
	r := newRepo(t)
	res, err := r.build(Options{})
	if err != nil || !res.Skipped || !res.Done {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(strings.Join(res.Lines, "\n"), "запуск сборки пропущен") {
		t.Error(res.Lines)
	}
}

func TestBuildRefusals(t *testing.T) {
	r := newRepo(t)
	if _, err := r.build(Options{Points: []conf.BuildPoint{{Dir: "nope/", Cmd: "true"}}}); err == nil || !strings.Contains(err.Error(), "нет папки") {
		t.Errorf("нет папки: %v", err)
	}
	r.git("checkout", "-q", "-b", "feat")
	if _, err := r.build(Options{}); err == nil || !strings.Contains(err.Error(), "перейди") {
		t.Errorf("не на dev: %v", err)
	}
	r.git("checkout", "-q", "dev")
	os.WriteFile(filepath.Join(r.dir, "a.txt"), []byte("dirty\n"), 0o644)
	if _, err := r.build(Options{}); err == nil || !strings.Contains(err.Error(), "незакоммиченные") {
		t.Errorf("грязное дерево: %v", err)
	}
	r.git("checkout", "-q", "--", "a.txt")
	if _, err := r.build(Options{State: 'x'}); err == nil {
		t.Error("плохое состояние принято")
	}
	if _, err := r.build(Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.build(Options{Now: t0.Add(time.Hour)}); err == nil || !strings.Contains(err.Error(), "уже снята") {
		t.Errorf("повторная сборка того же B: %v", err)
	}
}

func TestBuildNoNamedCommit(t *testing.T) {
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", Dev)
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.commit("a.txt", "a\n", "chore: init")
	if _, err := r.build(Options{}); err == nil || !strings.Contains(err.Error(), "нет именованных") {
		t.Errorf("%v", err)
	}
}

func TestBuildDryChangesNothing(t *testing.T) {
	r := newRepo(t)
	res, err := r.build(Options{Dry: true, Points: []conf.BuildPoint{{Dir: "./", Cmd: "touch ran.txt"}}})
	if err != nil || res.Done || len(res.Runs) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "ran.txt")); err == nil {
		t.Error("dry запустил команду")
	}
	if r.git("branch", "--list", "build/*") != "" {
		t.Error("dry создал ветку")
	}
}

func TestBuildVersionInEnv(t *testing.T) {
	r := newRepo(t)
	tip := r.git("rev-parse", "--short=7", "HEAD")
	res, err := r.build(Options{Points: []conf.BuildPoint{{Dir: "./", Cmd: "printf %s \"$SKY_BUILD_VERSION\" > ver.out"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(r.dir, "ver.out"))
	if string(b) != "v1.a1.29+29."+tip || !strings.HasSuffix(res.Message, string(b)) {
		t.Errorf("SKY_BUILD_VERSION = %q", b)
	}
}
