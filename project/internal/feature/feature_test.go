package feature

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
| cache | кэш | leaf |
| main | зерно | cache |
`

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
	os.WriteFile(p, []byte(body), 0o644)
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
	r.commit("docs/PROJECT-GRAPH.md", graphDoc, "docs: graph")
	r.commit("a.txt", "a\n", "Add y | v1.1.29+29.abc1234")
	return r
}

var t0 = time.Date(2026, 11, 25, 10, 5, 0, 0, time.UTC)

func (r repo) run(o Options) (Result, error) {
	o.Root = r.dir
	if o.Now.IsZero() {
		o.Now = t0
	}
	return Run(o)
}

func TestModuleTargetIsRemembered(t *testing.T) {
	r := newRepo(t)
	res, err := r.run(Options{Name: "add-cache", Module: "cache"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "wip/1125-1005/add-cache" || r.git("symbolic-ref", "--short", "HEAD") != res.Branch {
		t.Fatalf("ветка: %+v", res)
	}
	tg, ok := Load(r.dir, res.Branch)
	if !ok || tg.Kind != Module || tg.Module != "cache" || strings.Join(tg.Chain, ",") != "cache" || tg.Base != Dev {
		t.Fatalf("цель: %+v %v", tg, ok)
	}
	b, cur, ok := Current(r.dir)
	if !ok || b != res.Branch || cur.Module != "cache" {
		t.Fatalf("Current: %q %+v", b, cur)
	}
}

func TestSeedAndNoTarget(t *testing.T) {
	r := newRepo(t)
	res, err := r.run(Options{Name: "tweak", Seed: true})
	if err != nil {
		t.Fatal(err)
	}
	if tg, ok := Load(r.dir, res.Branch); !ok || tg.Kind != Seed {
		t.Fatalf("seed: %+v %v", tg, ok)
	}
	r.git("checkout", "-q", Dev)
	res, err = r.run(Options{Name: "add-cache-layer", Now: t0.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(r.dir, res.Branch); ok {
		t.Error("без цели ничего не запоминается")
	}
	out := strings.Join(res.Lines, "\n")
	if !strings.Contains(out, "sky feature add-cache-layer --module cache") || !strings.Contains(out, "модули проекта: leaf, cache, main") {
		t.Errorf("подсказка: %s", out)
	}
}

func TestFixTargetBranchesFromBuild(t *testing.T) {
	r := newRepo(t)
	bres, err := build.Exec(build.Options{Root: r.dir, Now: t0})
	if err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "-q", Dev)
	res, err := r.run(Options{Name: "null-check", Fix: true, Now: t0.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Base != bres.Branch || r.git("rev-parse", "HEAD") != r.git("rev-parse", bres.Branch) {
		t.Fatalf("ветка исправления не от сборки: %+v", res)
	}
	if tg, ok := Load(r.dir, res.Branch); !ok || tg.Kind != Fix || tg.Base != bres.Branch {
		t.Fatalf("цель: %+v %v", tg, ok)
	}
}

func TestRefusalsAndDry(t *testing.T) {
	r := newRepo(t)
	for _, c := range []struct {
		o    Options
		want string
	}{
		{Options{Name: "Bad Name"}, "имя фичи"},
		{Options{Name: "x", Fix: true, Seed: true}, "несовместим"},
		{Options{Name: "x", Chain: []string{"a"}}, "вместе с --module"},
		{Options{Name: "x", Build: "b"}, "вместе с --fix"},
		{Options{Name: "x", Module: "nope"}, "нет в графе"},
		{Options{Name: "x", Fix: true}, "сборок нет"},
	} {
		if _, err := r.run(c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c.o, err)
		}
	}
	before := r.git("branch", "--list")
	res, err := r.run(Options{Name: "dry", Module: "cache", Dry: true})
	if err != nil || res.Branch == "" || r.git("branch", "--list") != before {
		t.Fatalf("dry меняет: %+v %v", res, err)
	}
	os.WriteFile(filepath.Join(r.dir, "a.txt"), []byte("dirty\n"), 0o644)
	if _, err := r.run(Options{Name: "x"}); err == nil || !strings.Contains(err.Error(), "незакоммиченные") {
		t.Errorf("грязное дерево: %v", err)
	}
}
