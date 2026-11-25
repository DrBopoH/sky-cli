package land

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DrBopoH/sky-cli/internal/build"
)

const graphDoc = `# G

| Модуль | Ответственность | Зависит от |
|---|---|---|
| leaf | лист | — |
| mid | середина | leaf |
| a | один | leaf |
| b | два | leaf |
| main | зерно | mid, a, b |
`

type repo struct {
	t   *testing.T
	dir string
}

func (r repo) git(args ...string) string {
	r.t.Helper()
	c := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	out, err := c.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r repo) write(name, body string) {
	r.t.Helper()
	p := filepath.Join(r.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r repo) commit(name, body, msg string) {
	r.t.Helper()
	r.write(name, body)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func newRepo(t *testing.T) repo {
	t.Helper()
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", Dev)
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.git("config", "commit.gpgsign", "false")
	r.write(GraphPath, graphDoc)
	r.commit("base.txt", "0\n", "chore: init")
	return r
}

func (r repo) feature(name string, files ...string) {
	r.t.Helper()
	r.git("checkout", "-q", "-b", name, Dev)
	for _, f := range files {
		r.commit(f, f+"\n", "feat: "+f)
	}
}

func (r repo) subject(rev string) string { return r.git("log", "-1", "--format=%s", rev) }

var t0 = time.Date(2026, 11, 25, 10, 20, 0, 0, time.UTC)

func (r repo) land(o Options) (Result, error) {
	o.Root = r.dir
	if o.Now.IsZero() {
		o.Now = t0
	}
	return Run(o)
}

func TestSeedSquash(t *testing.T) {
	r := newRepo(t)
	r.feature("thing", "x.txt", "y.txt")
	tip := r.git("rev-parse", "--short=7", "HEAD")
	res, err := r.land(Options{Summary: "Add thing", Seed: true, From: "1.1.35+35"})
	if err != nil {
		t.Fatal(err)
	}
	want := "Add thing | v1.1.36+36." + tip
	if res.Message != want || r.subject(Dev) != want || r.git("symbolic-ref", "--short", "HEAD") != Dev {
		t.Fatalf("%q / %q", res.Message, r.subject(Dev))
	}
	if r.git("rev-list", "--count", Dev) != "2" {
		t.Fatal("squash: в dev один новый коммит")
	}
	// вторая фича: версия берётся из истории dev, From не нужен
	r.feature("two", "z.txt")
	if _, err := r.land(Options{Summary: "Add two", Seed: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.subject(Dev), "Add two | v1.1.37+37.") {
		t.Fatal(r.subject(Dev))
	}
}

func TestModuleChainAndMergeUp(t *testing.T) {
	r := newRepo(t)
	r.feature("f1", "m.txt")
	res, err := r.land(Options{Summary: "Add mid part", Module: "mid", Chain: []string{"mid"}, From: "1.1.0+0"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Message, "Add mid part | v1.") || strings.Contains(res.Message, "+") {
		t.Fatal(res.Message)
	}
	br := "mid/1125-1020"
	if res.Branch != br || r.git("config", "branch."+br+".sky-parent") != Dev || r.git("config", "branch."+br+".sky-state") != "open" {
		t.Fatalf("branch %q", res.Branch)
	}
	// вторая фича в тот же модуль: ветка открытая, номер растёт
	r.feature("f2", "n.txt")
	res2, err := r.land(Options{Summary: "More mid", Module: "mid", From: "1.1.0+0"})
	if err != nil || !strings.HasPrefix(res2.Message, "More mid | v2.") {
		t.Fatalf("%q %v", res2.Message, err)
	}
	// слияние вверх в dev: C = 0 + 2 именованных + 1 = 3, T = 3
	r.git("checkout", "-q", br)
	up, err := r.land(Options{Summary: "Mid done", From: "1.1.0+0"})
	if err != nil || !strings.HasPrefix(up.Message, "Mid done | v1.1.3+3.") {
		t.Fatalf("%q %v", up.Message, err)
	}
	if r.git("config", "branch."+br+".sky-state") != "merged" || r.git("symbolic-ref", "--short", "HEAD") != Dev {
		t.Fatal("после слияния ветка merged, HEAD на dev")
	}
	if r.git("rev-list", "--count", "--merges", Dev) != "1" {
		t.Fatal("слияние — merge-коммит")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "m.txt")); err != nil {
		t.Fatal("файлы фич попали в dev")
	}
}

// пример регламента 3.5.6: dev на 27, две параллельные ветки по одному коммиту дают слияния 29 и 31.
func TestParallelBranchesRegulationExample(t *testing.T) {
	r := newRepo(t)
	r.feature("fa", "a.txt")
	if _, err := r.land(Options{Summary: "Part a", Module: "a", From: "1.1.27+27"}); err != nil {
		t.Fatal(err)
	}
	r.feature("fb", "b.txt")
	if _, err := r.land(Options{Summary: "Part b", Module: "b", From: "1.1.27+27"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.subject("a/1125-1020"), "| v28.") || !strings.Contains(r.subject("b/1125-1020"), "| v28.") {
		t.Fatalf("на ветках одинаковые номера: %s / %s", r.subject("a/1125-1020"), r.subject("b/1125-1020"))
	}
	r.git("checkout", "-q", "a/1125-1020")
	if res, err := r.land(Options{Summary: "Merge a", From: "1.1.27+27"}); err != nil || !strings.HasPrefix(res.Message, "Merge a | v1.1.29+29.") {
		t.Fatalf("%q %v", res.Message, err)
	}
	r.git("checkout", "-q", "b/1125-1020")
	if res, err := r.land(Options{Summary: "Merge b", From: "1.1.27+27"}); err != nil || !strings.HasPrefix(res.Message, "Merge b | v1.1.31+31.") {
		t.Fatalf("%q %v", res.Message, err)
	}
}

func TestNestedChainBottomUp(t *testing.T) {
	r := newRepo(t)
	r.feature("f", "l.txt")
	if _, err := r.land(Options{Summary: "Leaf part", Module: "leaf", Chain: []string{"mid", "leaf"}, From: "1.1.0+0"}); err != nil {
		t.Fatal(err)
	}
	leaf, mid := "leaf/1125-1020", "mid/1125-1020"
	if r.git("config", "branch."+leaf+".sky-parent") != mid || r.git("config", "branch."+mid+".sky-parent") != Dev {
		t.Fatal("ветки вложены по цепочке")
	}
	// верхнюю ветку нельзя влить, пока открыта нижняя
	r.git("checkout", "-q", mid)
	r.write("mid.txt", "m\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "x | v1.abc1234")
	if _, err := r.land(Options{Summary: "Too early"}); err == nil || !strings.Contains(err.Error(), "вложенные") {
		t.Fatalf("ждали отказ про вложенные ветки: %v", err)
	}
	r.git("checkout", "-q", leaf)
	res, err := r.land(Options{Summary: "Leaf up"})
	if err != nil || !strings.Contains(res.Message, "| v") || strings.Contains(res.Message, "+") || res.Branch != mid {
		t.Fatalf("%q %q %v", res.Message, res.Branch, err)
	}
	res, err = r.land(Options{Summary: "Mid up", From: "1.1.0+0"})
	if err != nil || !strings.Contains(res.Message, "+") || res.Branch != Dev {
		t.Fatalf("%q %v", res.Message, err)
	}
}

func TestAmbiguousChainAndErrors(t *testing.T) {
	r := newRepo(t)
	r.feature("f", "l.txt")
	_, err := r.land(Options{Summary: "Leaf", Module: "leaf"})
	if err == nil || !strings.Contains(err.Error(), "--chain a,leaf") && !strings.Contains(err.Error(), "--chain mid,leaf") {
		t.Fatalf("ждали список цепочек: %v", err)
	}
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"нет резюме", Options{Summary: " ", Seed: true}, "резюме"},
		{"длинное", Options{Summary: strings.Repeat("я", 73), Seed: true}, "длиннее"},
		{"нет модуля", Options{Summary: "x", Module: "ghost"}, "нет в графе"},
		{"зерно как модуль", Options{Summary: "x", Module: "main"}, "--seed"},
		{"нужен флаг", Options{Summary: "x"}, "--module"},
		{"цепочка не по графу", Options{Summary: "x", Module: "leaf", Chain: []string{"a", "mid", "leaf"}}, "не по графу"},
	}
	for _, c := range cases {
		if _, err := r.land(c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// ничего не создано
	if r.git("branch", "--list", "*/1125-1020") != "" {
		t.Fatal("отказ ничего не меняет")
	}
}

func TestDirtyAndDevRefused(t *testing.T) {
	r := newRepo(t)
	if _, err := r.land(Options{Summary: "x", Seed: true}); err == nil || !strings.Contains(err.Error(), "нечего вливать") {
		t.Fatalf("на dev: %v", err)
	}
	r.feature("f", "a.txt")
	r.write("a.txt", "правка\n")
	if _, err := r.land(Options{Summary: "x", Seed: true}); err == nil || !strings.Contains(err.Error(), "незакоммиченные") {
		t.Fatalf("грязное дерево: %v", err)
	}
}

func TestDryChangesNothing(t *testing.T) {
	r := newRepo(t)
	r.feature("f", "a.txt")
	before := r.git("rev-parse", Dev)
	res, err := r.land(Options{Summary: "Dry run", Module: "a", Dry: true})
	if err != nil || res.Message == "" || len(res.Lines) == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if r.git("rev-parse", Dev) != before || r.git("branch", "--list", "*/1125-1020") != "" || r.git("symbolic-ref", "--short", "HEAD") != "f" {
		t.Fatal("dry ничего не меняет")
	}
}

func TestConflictRestores(t *testing.T) {
	r := newRepo(t)
	r.feature("f1", "same.txt")
	r.git("checkout", "-q", "-b", "f2", Dev)
	r.commit("same.txt", "другое\n", "feat: same other")
	r.git("checkout", "-q", "f1")
	if _, err := r.land(Options{Summary: "First", Seed: true, From: "1.1.0+0"}); err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "-q", "f2")
	before := r.git("rev-parse", Dev)
	if _, err := r.land(Options{Summary: "Second", Seed: true}); err == nil || !strings.Contains(err.Error(), "ничего не изменено") {
		t.Fatalf("ждали отказ из-за конфликта: %v", err)
	}
	if r.git("rev-parse", Dev) != before || r.git("symbolic-ref", "--short", "HEAD") != "f2" || r.git("status", "--porcelain") != "" {
		t.Fatal("после конфликта всё как было")
	}
}

func TestAfterBuildNextIsBPlusOne(t *testing.T) {
	r := newRepo(t)
	r.feature("one", "x.txt")
	if _, err := r.land(Options{Summary: "Add one", Seed: true, From: "1.1.28+28"}); err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "-q", Dev)
	if _, err := build.Exec(build.Options{Root: r.dir, From: "1.1.28+28", Now: t0}); err != nil {
		t.Fatal(err)
	}
	// после сборки: B+1, C=1, T продолжается
	r.feature("two", "y.txt")
	res, err := r.land(Options{Summary: "Add two", Seed: true, From: "1.1.28+28"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, " | v1.2.1+30.") {
		t.Errorf("после сборки ждали v1.2.1+30, получено %s", res.Message)
	}
}

func TestAfterBuildModuleMerge(t *testing.T) {
	r := newRepo(t)
	r.feature("one", "x.txt")
	if _, err := r.land(Options{Summary: "Add one", Seed: true, From: "1.1.28+28"}); err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "-q", Dev)
	if _, err := build.Exec(build.Options{Root: r.dir, From: "1.1.28+28", Now: t0}); err != nil {
		t.Fatal(err)
	}
	r.feature("f", "m.txt")
	if _, err := r.land(Options{Summary: "Add m", Module: "mid", Chain: []string{"mid"}, From: "1.1.28+28", Now: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	res, err := r.land(Options{Summary: "Leaf done", From: "1.1.28+28", Now: t0.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, " | v1.2.2+31.") {
		t.Errorf("ждали v1.2.2+31, получено %s", res.Message)
	}
}

func TestBuildAfterForkPointKeepsOldCount(t *testing.T) {
	// ветка модуля открыта до сборки, источник сборки новее её точки ответвления: счёт ветки не сбрасывается
	r := newRepo(t)
	r.commit("n.txt", "n\n", "Add n | v1.1.28+28.aaa1111")
	r.feature("f", "m.txt")
	if _, err := r.land(Options{Summary: "Add m", Module: "mid", Chain: []string{"mid"}}); err != nil {
		t.Fatal(err)
	}
	r.feature("one", "x.txt")
	if _, err := r.land(Options{Summary: "Add one", Seed: true}); err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "-q", Dev)
	if _, err := build.Exec(build.Options{Root: r.dir, Now: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	r.feature("g", "k.txt")
	res, err := r.land(Options{Summary: "Add k", Module: "mid", Chain: []string{"mid"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, " | v30.") {
		t.Errorf("ждали v30.hash (28 + 2 в ветке), получено %s", res.Message)
	}
}
