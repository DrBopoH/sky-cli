package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newRepo(t *testing.T) Layout {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git недоступен: %v %s", err, out)
	}
	l, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.MkDirs(); err != nil {
		t.Fatal(err)
	}
	return l
}

func status(t *testing.T, ch []Change, path string) Status {
	t.Helper()
	for _, c := range ch {
		if c.Path == path {
			return c.Status
		}
	}
	t.Fatalf("нет записи для %s в %v", path, ch)
	return Same
}

var set = []Generated{{Name: "a.md", Content: []byte("a\n")}, {Name: "b.md", Content: []byte("b\n")}}

func TestFindNotRepo(t *testing.T) {
	if _, err := Find(t.TempDir()); err == nil {
		t.Fatal("ждали ошибку вне git")
	}
}

func TestMkDirs(t *testing.T) {
	l := newRepo(t)
	for _, d := range []string{l.Stages, l.Readme, l.Foryou, l.Clouds} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("нет папки %s", d)
		}
	}
}

func TestSyncReadmeLifecycle(t *testing.T) {
	l := newRepo(t)
	ch, err := l.SyncReadme(set)
	if err != nil || status(t, ch, ".sky/readme/a.md") != Created || status(t, ch, ".sky/readme/b.md") != Created {
		t.Fatalf("создание: %v %v", ch, err)
	}
	a := filepath.Join(l.Readme, "a.md")
	if fi, _ := os.Stat(a); fi.Mode().Perm() != 0o444 {
		t.Fatalf("права %v, нужно 0444", fi.Mode().Perm())
	}
	ch, _ = l.SyncReadme(set)
	if status(t, ch, ".sky/readme/a.md") != Same {
		t.Fatalf("повтор: %v", ch)
	}

	os.Chmod(a, 0o644)
	ch, _ = l.SyncReadme(set)
	if status(t, ch, ".sky/readme/a.md") != Replaced {
		t.Fatalf("права сняты: %v", ch)
	}

	os.Chmod(a, 0o644)
	os.WriteFile(a, []byte("правка"), 0o644)
	ch, _ = l.SyncReadme(set)
	if b, _ := os.ReadFile(a); status(t, ch, ".sky/readme/a.md") != Replaced || string(b) != "a\n" {
		t.Fatalf("правка не откатилась: %v %q", ch, b)
	}

	os.Remove(a)
	ch, _ = l.SyncReadme(set)
	if status(t, ch, ".sky/readme/a.md") != Created {
		t.Fatalf("удалён: %v", ch)
	}
}

func TestSyncReadmePrunesStray(t *testing.T) {
	l := newRepo(t)
	os.WriteFile(filepath.Join(l.Readme, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(l.Readme, "dir"), 0o755)
	mine := filepath.Join(l.Clouds, "notes.md")
	os.WriteFile(mine, []byte("мои заметки"), 0o644)

	ch, err := l.SyncReadme(set)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".sky/readme/stray.md", ".sky/readme/dir"} {
		if status(t, ch, p) != Removed {
			t.Errorf("%s должен быть удалён", p)
		}
	}
	if _, err := os.Stat(mine); err != nil {
		t.Error("заметки в clouds/ трогать нельзя")
	}
	entries, _ := os.ReadDir(l.Readme)
	if len(entries) != 2 {
		t.Fatalf("в readme/ остались лишние файлы: %v", entries)
	}
}

func TestForyouAndCloudsUntouched(t *testing.T) {
	l := newRepo(t)
	f := filepath.Join(l.Foryou, "status.md")
	os.WriteFile(f, []byte("факты"), 0o444)
	l.SyncReadme(set)
	if b, err := os.ReadFile(f); err != nil || string(b) != "факты" {
		t.Fatal("SyncReadme не должен трогать foryou/")
	}
}

func TestEnsureIgnored(t *testing.T) {
	l := newRepo(t)
	for i, want := range []bool{true, false} {
		got, err := l.EnsureIgnored(".sky/")
		if err != nil || got != want {
			t.Fatalf("вызов %d: %v %v", i, got, err)
		}
	}
	os.WriteFile(l.Root+"/.gitignore", []byte("bin\n.sky"), 0o644)
	if got, _ := l.EnsureIgnored(".sky/"); got {
		t.Fatal("вариант без слэша должен считаться уже заданным")
	}
	os.WriteFile(l.Root+"/.gitignore", []byte("bin"), 0o644)
	l.EnsureIgnored(".sky/")
	if b, _ := os.ReadFile(l.Root + "/.gitignore"); string(b) != "bin\n.sky/\n" {
		t.Fatalf("%q", b)
	}
}
