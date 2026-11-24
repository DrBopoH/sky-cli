package look

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/conf"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func repoDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	sh(t, d, "init", "-q", "-b", "main")
	return d
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(files []File, name string) string {
	for _, f := range files {
		if f.Name == name {
			return f.Content
		}
	}
	return "<нет файла " + name + ">"
}

func input(root string) Input {
	return Input{Root: root, Stages: filepath.Join(root, ".sky", "stages"), Conf: conf.Default(), Inbox: "~/Downloads"}
}

func TestEmptyRepo(t *testing.T) {
	d := repoDir(t)
	fs := Build(input(d))
	if len(fs) != 6 {
		t.Fatalf("файлов: %d", len(fs))
	}
	for _, f := range fs { // пустой репозиторий не ломает ни один файл
		if strings.TrimSpace(f.Content) == "" {
			t.Errorf("%s пуст", f.Name)
		}
	}
}

func TestStatusFacts(t *testing.T) {
	d := repoDir(t)
	write(t, d, "a.txt", "a\n")
	write(t, d, ".gitignore", "*.db\n")
	sh(t, d, "add", "-A")
	sh(t, d, "commit", "-q", "-m", "feat: first")
	write(t, d, "a.txt", "b\n")
	write(t, d, "new.txt", "n\n")
	write(t, d, "x.db", "secret")
	sh(t, d, "remote", "add", "origin", "https://user:token@example.org/r.git")
	in := input(d)
	in.Patches = []string{"patch-1.zip", "patch-2.zip"}
	s := get(Build(in), "status.md")
	for _, want := range []string{"main", "feat: first", "M a.txt", "? new.txt", "patch-1.zip", "patch-2.zip", "https://example.org/r.git"} {
		if !strings.Contains(s, want) {
			t.Errorf("нет %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "token") || strings.Contains(s, "x.db") {
		t.Errorf("утечка секрета или игнорируемого файла:\n%s", s)
	}
}

func TestHistoryLimitsPerBranch(t *testing.T) {
	d := repoDir(t)
	for i := 0; i < MaxBranchCommits+5; i++ {
		sh(t, d, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("chore: c%d", i))
	}
	sh(t, d, "branch", "other")
	h := get(Build(input(d)), "history.md")
	if n := strings.Count(h, "chore: c"); n != 2*MaxBranchCommits {
		t.Errorf("коммитов в файле: %d, ждали %d (по %d на ветку)", n, 2*MaxBranchCommits, MaxBranchCommits)
	}
	if !strings.Contains(h, "main") || !strings.Contains(h, "other") {
		t.Errorf("обе ветки должны быть в файле:\n%.300s", h)
	}
	if strings.Contains(h, "chore: c0\n") {
		t.Error("самый старый коммит должен быть обрезан")
	}
}

func TestTreeAndStructure(t *testing.T) {
	d := repoDir(t)
	write(t, d, "README.md", "r")
	write(t, d, "project/main.go", "m")
	write(t, d, "project/internal/a/a.go", "a")
	write(t, d, "project/internal/a/b.go", "b")
	sh(t, d, "add", "-A")
	sh(t, d, "commit", "-q", "-m", "feat: x")
	os.Remove(filepath.Join(d, "project", "main.go")) // удалён с диска, ещё в индексе
	fs := Build(input(d))
	tr := get(fs, "tree.md")
	want := "README.md\nproject/\n  internal/\n    a/\n      a.go\n      b.go\n"
	if !strings.Contains(tr, want) || strings.Contains(tr, "main.go") {
		t.Errorf("tree:\n%s", tr)
	}
}

func TestGraph(t *testing.T) {
	d := repoDir(t)
	sh(t, d, "commit", "-q", "--allow-empty", "-m", "chore: base")
	sh(t, d, "switch", "-q", "-c", "dev")
	sh(t, d, "commit", "-q", "--allow-empty", "-m", "feat: on dev")
	sh(t, d, "switch", "-q", "main")
	sh(t, d, "commit", "-q", "--allow-empty", "-m", "fix: on main")
	sh(t, d, "merge", "-q", "--no-ff", "-m", "chore: merge dev", "dev")
	g := get(Build(input(d)), "graph.md")
	for _, want := range []string{"(HEAD -> main)", "chore: merge dev", "feat: on dev", "fix: on main", "chore: base", "|\\"} {
		if !strings.Contains(g, want) {
			t.Errorf("нет %q:\n%s", want, g)
		}
	}
}

func TestGraphEmpty(t *testing.T) {
	if g := get(Build(input(repoDir(t))), "graph.md"); strings.TrimSpace(g) == "" {
		t.Error("graph.md пуст")
	}
}

func TestTreeCap(t *testing.T) {
	d := repoDir(t)
	for i := 0; i < MaxTreeLines+10; i++ {
		write(t, d, fmt.Sprintf("f%04d.txt", i), "x")
	}
	tr := get(Build(input(d)), "tree.md")
	if n := strings.Count(tr, ".txt\n"); n != MaxTreeLines {
		t.Errorf("строк с файлами: %d", n)
	}
	if strings.Contains(tr, "f0000.txt\n") == false || strings.Contains(tr, fmt.Sprintf("f%04d.txt\n", MaxTreeLines+9)) {
		t.Error("видна должна быть только первая часть списка")
	}
}

func TestBranchesMarkCurrent(t *testing.T) {
	d := repoDir(t)
	sh(t, d, "commit", "-q", "--allow-empty", "-m", "chore: x")
	sh(t, d, "branch", "dev")
	b := get(Build(input(d)), "branches.md")
	if !strings.Contains(b, "* main  ") || !strings.Contains(b, "  dev  ") {
		t.Errorf("branches:\n%s", b)
	}
}
