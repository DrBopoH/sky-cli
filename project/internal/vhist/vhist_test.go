package vhist

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, name, msg string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", msg)
}

func TestLastBuildsEffective(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "t")
	git(t, dir, "config", "user.email", "t@t")
	h := H{Root: dir}
	if v, found, err := h.Last("dev"); err == nil && found {
		t.Fatalf("пустой репозиторий: %v", v)
	}
	commit(t, dir, "a", "Add a | v1.1.5+5.aaa1111")
	commit(t, dir, "b", "Add b | v1.1.6+6.bbb2222")
	v, found, err := h.Last("dev")
	if err != nil || !found || v.C != 6 || v.B != 1 {
		t.Fatalf("Last: %+v %v %v", v, found, err)
	}
	if _, ok := h.BuildOf(1, 1); ok {
		t.Fatal("сборки ещё нет")
	}
	tip := git(t, dir, "rev-parse", "dev")
	short := git(t, dir, "rev-parse", "--short=7", "dev")
	tree := git(t, dir, "rev-parse", "dev^{tree}")
	tail := git(t, dir, "commit-tree", tree, "-p", tip, "-m", "Build 1 alpha | v1.a1.6+6."+short)
	git(t, dir, "branch", "build/1125-1020", tail)
	b, ok := h.BuildOf(1, 1)
	if !ok || b.Source != tip || h.Branch(b) != "build/1125-1020" {
		t.Fatalf("BuildOf: %+v %v", b, ok)
	}
	// после сборки: B+1, C=0, T продолжается
	e, _, _ := h.Effective("dev")
	if e.B != 2 || e.C != 0 || e.T != 6 {
		t.Fatalf("Effective: %+v", e)
	}
	// точка до источника сборки не сдвигается
	e, _, _ = h.Effective("dev~1")
	if e.B != 1 || e.C != 5 {
		t.Fatalf("Effective до источника: %+v", e)
	}
	// после нового именованного коммита база снова обычная
	commit(t, dir, "c", "Add c | v1.2.1+7.ccc3333")
	e, _, _ = h.Effective("dev")
	if e.B != 2 || e.C != 1 {
		t.Fatalf("после нового коммита: %+v", e)
	}
}
