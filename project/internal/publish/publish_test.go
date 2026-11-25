package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// newRepo — main со стартовым коммитом, от него dev с двумя именованными, на dev хвост сборки 1 и исправление.
func newRepo(t *testing.T) repo {
	t.Helper()
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.git("config", "commit.gpgsign", "false")
	r.commit("start.txt", "s\n", "Init")
	r.git("checkout", "-q", "-b", "dev")
	r.commit("a.txt", "a\n", "Add x | v1.1.1+1.aaa1111")
	r.commit("b.txt", "b\n", "Add y | v1.1.2+2.bbb2222")
	tip := r.git("rev-parse", "HEAD")
	short := r.git("rev-parse", "--short=7", "HEAD")
	tree := r.git("rev-parse", "HEAD^{tree}")
	tail := r.git("commit-tree", tree, "-p", tip, "-m", "Build 1 alpha | v1.a1.2+2."+short)
	r.git("branch", "build/1125-1020", tail)
	return r
}

func TestPublishCreatesOneCommitOnMain(t *testing.T) {
	r := newRepo(t)
	mainTip := r.git("rev-parse", "main")
	res, err := Run(Options{Root: r.dir})
	if err != nil || !res.Done || res.Build != "build/1125-1020" {
		t.Fatalf("%v %+v", err, res)
	}
	head := r.git("rev-parse", "--short=7", "build/1125-1020")
	want := "Build 1 alpha | v1.a1.2+2." + head
	if got := r.git("log", "-1", "--format=%s", "main"); got != want {
		t.Errorf("сообщение %q, нужно %q", got, want)
	}
	if r.git("rev-parse", "main~1") != mainTip || r.git("rev-list", "--count", "main") != "2" {
		t.Error("коммит должен лечь одним поверх прежнего main")
	}
	if r.git("rev-parse", "main^{tree}") != r.git("rev-parse", "build/1125-1020^{tree}") {
		t.Error("дерево main не равно дереву сборки")
	}
	if r.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("HEAD должен остаться на dev")
	}
}

func TestPublishTwiceNoConflict(t *testing.T) {
	r := newRepo(t)
	if _, err := Run(Options{Root: r.dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: r.dir}); err == nil || !strings.Contains(err.Error(), "уже") {
		t.Fatalf("то же дерево второй раз: %v", err)
	}
	r.commit("c.txt", "c\n", "Add z | v1.2.1+3.ccc3333")
	tip, short, tree := r.git("rev-parse", "HEAD"), r.git("rev-parse", "--short=7", "HEAD"), r.git("rev-parse", "HEAD^{tree}")
	r.git("branch", "build/1125-1100", r.git("commit-tree", tree, "-p", tip, "-m", "Build 2 stable | v1.s2.1+3."+short))
	res, err := Run(Options{Root: r.dir, Build: "1125-1100"})
	if err != nil || !res.Done || r.git("rev-list", "--count", "main") != "3" {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.HasPrefix(r.git("log", "-1", "--format=%s", "main"), "Build 2 stable | v1.s2.1+3.") {
		t.Error(r.git("log", "-1", "--format=%s", "main"))
	}
}

func TestPublishUsesFixHeadAndTitle(t *testing.T) {
	r := newRepo(t)
	r.git("checkout", "-q", "build/1125-1020")
	r.commit("fix.txt", "f\n", "Fix crash | v1.a1-1.2+3.fff4444")
	r.git("checkout", "-q", "dev")
	head := r.git("rev-parse", "--short=7", "build/1125-1020")
	res, err := Run(Options{Root: r.dir, Title: "Release"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != "Release | v1.a1-1.2+3."+head {
		t.Errorf("%q", res.Message)
	}
}

func TestPublishRefusals(t *testing.T) {
	r := newRepo(t)
	r.git("checkout", "-q", "main")
	if _, err := Run(Options{Root: r.dir}); err == nil || !strings.Contains(err.Error(), "HEAD на main") {
		t.Fatalf("HEAD на main: %v", err)
	}
	r.git("checkout", "-q", "dev")
	if _, err := Run(Options{Root: r.dir, Build: "1125-9999"}); err == nil {
		t.Fatal("нет такой сборки")
	}
	if _, err := Run(Options{Root: r.dir, Title: strings.Repeat("я", 80)}); err == nil {
		t.Fatal("длинное название")
	}
	r.git("branch", "-m", "main", "trunk")
	if _, err := Run(Options{Root: r.dir}); err == nil || !strings.Contains(err.Error(), "нет ветки main") {
		t.Fatalf("нет main: %v", err)
	}
}

func TestPublishNoBuilds(t *testing.T) {
	r := newRepo(t)
	r.git("branch", "-D", "build/1125-1020")
	if _, err := Run(Options{Root: r.dir}); err == nil || !strings.Contains(err.Error(), "сборок нет") {
		t.Fatalf("%v", err)
	}
}

func TestPublishDryChangesNothing(t *testing.T) {
	r := newRepo(t)
	before := r.git("rev-parse", "main")
	res, err := Run(Options{Root: r.dir, Dry: true})
	if err != nil || res.Done || r.git("rev-parse", "main") != before || len(res.Lines) == 0 {
		t.Fatalf("%v %+v", err, res)
	}
}
