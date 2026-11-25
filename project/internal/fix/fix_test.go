package fix

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DrBopoH/sky-cli/internal/build"
	"github.com/DrBopoH/sky-cli/internal/vhist"
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
	os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o644)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// dev на 1.1.29+29, снята сборка build/1125-1020.
func newBuilt(t *testing.T) (repo, string) {
	t.Helper()
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", Dev)
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.git("config", "commit.gpgsign", "false")
	r.commit("a.txt", "a\n", "Add x | v1.1.28+28.aaa1111")
	r.commit("b.txt", "b\n", "Add y | v1.1.29+29.abc1234")
	res, err := build.Exec(build.Options{Root: r.dir, Now: time.Date(2026, 11, 25, 10, 20, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return r, res.Branch
}

func (r repo) fixBranch(from, name string, files ...string) {
	r.git("checkout", "-q", "-b", name, from)
	for _, f := range files {
		r.commit(f, f+"\n", "fix: "+f)
	}
}

func TestFixOnLatestBuild(t *testing.T) {
	r, br := newBuilt(t)
	r.fixBranch(br, "hot", "x.txt", "y.txt")
	res, err := Run(Options{Root: r.dir, Summary: "Fix crash"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != br || res.Branch != br {
		t.Fatalf("%+v", res)
	}
	if !strings.HasPrefix(res.Message, "Fix crash | v1.a1-1.29+30.") {
		t.Fatalf("версия: %s", res.Message)
	}
	if r.git("rev-list", "--count", "--merges", br) != "0" || r.git("rev-list", "--count", br+"~1.."+br) != "1" {
		t.Error("исправление — один squash-коммит")
	}
	// второе исправление: F=2, T=31
	r.fixBranch(br, "hot2", "z.txt")
	res, err = Run(Options{Root: r.dir, Summary: "Fix again"})
	if err != nil || !strings.HasPrefix(res.Message, "Fix again | v1.a1-2.29+31.") {
		t.Fatalf("%q %v", res.Message, err)
	}
}

func TestFixTCountsDevAndNextDevContinues(t *testing.T) {
	r, br := newBuilt(t)
	// dev ушёл вперёд: 1.2.1+30
	r.git("checkout", "-q", Dev)
	r.commit("n.txt", "n\n", "More | v1.2.1+30.bbb2222")
	r.fixBranch(br, "hot", "x.txt")
	res, err := Run(Options{Root: r.dir, Summary: "Fix late"})
	if err != nil || !strings.HasPrefix(res.Message, "Fix late | v1.a1-1.29+31.") {
		t.Fatalf("T исправления считается и от dev: %q %v", res.Message, err)
	}
	// следующее изменение dev продолжает T после исправления: 32
	r.git("checkout", "-q", Dev)
	v, _, err := (vhist.H{Root: r.dir}).Effective(Dev)
	if err != nil || v.T != 31 {
		t.Fatalf("Effective: %+v %v", v, err)
	}
}

func TestFixPickBuildAndRefusals(t *testing.T) {
	r, br := newBuilt(t)
	r.fixBranch(br, "hot", "x.txt")
	if _, err := Run(Options{Root: r.dir, Summary: "x", Build: "1125-1020", Dry: true}); err != nil {
		t.Fatalf("сборка по короткому имени: %v", err)
	}
	if _, err := Run(Options{Root: r.dir, Summary: "x", Build: "nope"}); err == nil || !strings.Contains(err.Error(), "не ветка сборки") {
		t.Errorf("%v", err)
	}
	if _, err := Run(Options{Root: r.dir}); err == nil {
		t.Error("пустое резюме принято")
	}
	r.git("checkout", "-q", Dev)
	if _, err := Run(Options{Root: r.dir, Summary: "x"}); err == nil || !strings.Contains(err.Error(), "отдельной ветке") {
		t.Errorf("на dev: %v", err)
	}
	r.git("checkout", "-q", br)
	if _, err := Run(Options{Root: r.dir, Summary: "x"}); err == nil || !strings.Contains(err.Error(), "ветке сборки") {
		t.Errorf("на ветке сборки: %v", err)
	}
	// ветка не от ветки сборки
	r.git("checkout", "-q", Dev)
	r.fixBranch(Dev, "wrong", "w.txt")
	if _, err := Run(Options{Root: r.dir, Summary: "x"}); err == nil || !strings.Contains(err.Error(), "не отведена") {
		t.Errorf("не от сборки: %v", err)
	}
}

func TestFixNoBuildsAndDry(t *testing.T) {
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", Dev)
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.commit("a.txt", "a\n", "Add | v1.1.1+1.aaa1111")
	r.git("checkout", "-q", "-b", "f")
	r.commit("f.txt", "f\n", "fix: f")
	if _, err := Run(Options{Root: r.dir, Summary: "x"}); err == nil || !strings.Contains(err.Error(), "сборок нет") {
		t.Errorf("%v", err)
	}
	r2, br := newBuilt(t)
	r2.fixBranch(br, "hot", "x.txt")
	before := r2.git("rev-parse", br)
	res, err := Run(Options{Root: r2.dir, Summary: "Dry", Dry: true})
	if err != nil || res.Message == "" || r2.git("rev-parse", br) != before || res.Branch != "hot" {
		t.Fatalf("dry меняет состояние: %+v %v", res, err)
	}
}
