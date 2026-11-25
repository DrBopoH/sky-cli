package modules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/graph"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

const reg = "| Модуль | Зависит от |\n|---|---|\n| leaf | — |\n| mid | leaf |\n| side | leaf |\n| top | mid, side |\n| main | top |\n"

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "t")
	git(t, dir, "config", "user.email", "t@t")
	git(t, dir, "config", "commit.gpgsign", "false")
	p := filepath.Join(dir, filepath.FromSlash(graph.Path))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(reg), 0o644)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "Add graph | v1.1.1+1.abc1234")
	return dir
}

func TestParseBranch(t *testing.T) {
	if m, s, ok := ParseBranch("storage/1125-1020"); !ok || m != "storage" || s != "1125-1020" {
		t.Fatalf("%q %q %v", m, s, ok)
	}
	for _, n := range []string{"dev", "wip/1125-1020/x", "a/b", "storage/1125-102"} {
		if _, _, ok := ParseBranch(n); ok {
			t.Errorf("%q — не ветка модуля", n)
		}
	}
}

func TestInspectAll(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "branch", "top/1125-1020")
	git(t, dir, "config", "branch.top/1125-1020.sky-state", "open")
	git(t, dir, "config", "branch.top/1125-1020.sky-parent", "dev")
	git(t, dir, "branch", "build/1125-1030")
	git(t, dir, "branch", "wip/1125-1030/feat")
	res, err := Inspect(dir, "")
	if err != nil || res.Warn != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	out := strings.Join(res.Lines, "\n")
	for _, want := range []string{"зерно: main", "уровень 3", "уровень 0", "top/1125-1020  [open] → dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("нет %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "build/") || strings.Contains(out, "wip/") {
		t.Errorf("сборки и wip не ветки модулей:\n%s", out)
	}
}

func TestInspectOne(t *testing.T) {
	dir := newRepo(t)
	res, err := Inspect(dir, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(res.Lines, "\n")
	for _, want := range []string{"leaf — уровень 0", "от него зависят: mid, side", "dev → top → mid → leaf", "dev → top → side → leaf", "открытых веток нет"} {
		if !strings.Contains(out, want) {
			t.Errorf("нет %q:\n%s", want, out)
		}
	}
	if res, _ := Inspect(dir, "main"); !strings.Contains(strings.Join(res.Lines, "\n"), "--seed") {
		t.Error("у зерна нет подсказки про --seed")
	}
	if _, err := Inspect(dir, "nope"); err == nil {
		t.Fatal("нет такого модуля")
	}
}

func TestInspectWarnsUnknownModuleBranch(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "branch", "ghost/1125-1020")
	res, err := Inspect(dir, "")
	if err != nil || res.Warn != 1 || !strings.Contains(strings.Join(res.Lines, "\n"), "заведена не sky land") {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestInspectNeedsGraph(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if _, err := Inspect(dir, ""); err == nil {
		t.Fatal("без реестра отчёта нет")
	}
}
