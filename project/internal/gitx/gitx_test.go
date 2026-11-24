package gitx

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if HasCommits(dir) {
		t.Fatal("в новом репозитории не должно быть коммитов")
	}
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t"}} {
		if _, err := Run(dir, "config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestToplevelAndOutsideRepo(t *testing.T) {
	dir := repo(t)
	top, err := Toplevel(dir)
	if err != nil || top == "" {
		t.Fatalf("Toplevel: %q %v", top, err)
	}
	if _, err := Toplevel(t.TempDir()); err == nil {
		t.Fatal("вне репозитория Toplevel обязан отказать")
	}
}

func TestBlobIgnoredUntracked(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitignore", "skip.txt\n", 0o644)
	write(t, dir, "a.sh", "echo a\n", 0o755)
	write(t, dir, "skip.txt", "x", 0o644)
	write(t, dir, "new.txt", "n", 0o644)
	if _, err := Run(dir, "add", ".gitignore", "a.sh"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(dir, "commit", "-q", "-m", "chore: base"); err != nil {
		t.Fatal(err)
	}
	if len(Head(dir)) != 7 || !HasCommits(dir) {
		t.Fatalf("Head: %q", Head(dir))
	}
	data, exe, ok := Blob(dir, "a.sh")
	if !ok || !exe || string(data) != "echo a\n" {
		t.Fatalf("Blob: %q %v %v", data, exe, ok)
	}
	if _, _, ok := Blob(dir, "new.txt"); ok {
		t.Fatal("нового файла в HEAD нет")
	}
	if ig, err := Ignored(dir, "skip.txt"); err != nil || !ig {
		t.Fatalf("skip.txt должен игнорироваться: %v %v", ig, err)
	}
	if ig, err := Ignored(dir, "new.txt"); err != nil || ig {
		t.Fatalf("new.txt не игнорируется: %v %v", ig, err)
	}
	un, err := Untracked(dir)
	if err != nil || !reflect.DeepEqual(un, []string{"new.txt"}) {
		t.Fatalf("Untracked: %v %v", un, err)
	}
	vis, _ := Visible(dir)
	sort.Strings(vis)
	if !reflect.DeepEqual(vis, []string{".gitignore", "a.sh", "new.txt"}) {
		t.Fatalf("Visible: %v", vis)
	}
	if Dirty(dir) != "" {
		t.Fatal("отслеживаемые файлы не менялись")
	}
	write(t, dir, "a.sh", "echo b\n", 0o755)
	if Dirty(dir) == "" {
		t.Fatal("правка отслеживаемого файла должна быть видна")
	}
}

func TestUnstageWithoutCommits(t *testing.T) {
	dir := repo(t)
	write(t, dir, "f.txt", "x", 0o644)
	if _, err := Run(dir, "add", "f.txt"); err != nil {
		t.Fatal(err)
	}
	Unstage(dir, []string{"f.txt"})
	if out := Try(dir, "diff", "--cached", "--name-only"); out != "" {
		t.Fatalf("индекс должен очиститься: %q", out)
	}
}
