package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DrBopoH/sky-cli/internal/repo"
)

func zipPatch(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// project: git-репозиторий с init и inbox во временной папке.
func project(t *testing.T) (dir, inbox string) {
	t.Helper()
	dir = gitDir(t)
	inbox = t.TempDir()
	if code, _, errs := run(New(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	conf := "scope = [project/]\ninbox = " + inbox + "\n"
	if err := os.WriteFile(repo.At(dir).Conf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, inbox
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func TestApplyBasic(t *testing.T) {
	dir, inbox := project(t)
	zipPatch(t, filepath.Join(inbox, "patch-one.zip"), map[string]string{
		"project/a.txt":       "a",
		".sky/clouds/plan.md": "план",
	})
	code, out, errs := run(New(), dir, "apply")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	for _, want := range []string{"── patch-one.zip", "+ project/a.txt", "+ .sky/clouds/plan.md", "● запись .sky/stages/history/0001-"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе нет %q:\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "project", "a.txt")); string(b) != "a" {
		t.Error("файл не записан")
	}
	if _, err := os.Stat(filepath.Join(inbox, "patch-one.zip")); err == nil {
		t.Error("патч должен уйти из inbox")
	}
	if code, _, errs := run(New(), dir, "apply"); code != 1 || !strings.Contains(errs, "нет patch*.zip") {
		t.Errorf("повторный запуск без патчей: %d %s", code, errs)
	}
}

func TestApplyOrderByTime(t *testing.T) {
	dir, inbox := project(t)
	older := filepath.Join(inbox, "patch-b.zip")
	newer := filepath.Join(inbox, "patch-a.zip")
	zipPatch(t, older, map[string]string{"x.txt": "старый"})
	zipPatch(t, newer, map[string]string{"x.txt": "новый"})
	now := time.Now()
	os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(newer, now, now)
	if code, _, errs := run(New(), dir, "apply"); code != 0 {
		t.Fatal(errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "x.txt")); string(b) != "новый" {
		t.Fatalf("позднее время должно перекрывать раннее: %q", b)
	}
}

func TestApplyDirtyTree(t *testing.T) {
	dir, inbox := project(t)
	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("1"), 0o644)
	commitAll(t, dir)
	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("правка руками"), 0o644)
	zipPatch(t, filepath.Join(inbox, "patch.zip"), map[string]string{"new.txt": "n"})

	if code, _, errs := run(New(), dir, "apply"); code != 1 || !strings.Contains(errs, "--force") {
		t.Fatalf("грязное дерево должно останавливать: %d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err == nil {
		t.Fatal("ничего не должно быть записано")
	}
	if code, _, errs := run(New(), dir, "apply", "--force"); code != 0 {
		t.Fatalf("--force: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Error("с --force патч должен примениться")
	}
	if code, _, errs := run(New(), dir, "apply", "--nope"); code != 1 || !strings.Contains(errs, "--nope") {
		t.Errorf("неизвестный аргумент: %d %s", code, errs)
	}
}

func TestApplyDeleteOperation(t *testing.T) {
	dir, inbox := project(t)
	os.MkdirAll(filepath.Join(dir, "old"), 0o755)
	os.WriteFile(filepath.Join(dir, "old", "f.txt"), []byte("x"), 0o644)
	commitAll(t, dir)
	zipPatch(t, filepath.Join(inbox, "patch.zip"), map[string]string{".sky/operation/delete": "old/f.txt\nnope.txt\n"})
	code, out, errs := run(New(), dir, "apply")
	if code != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "old", "f.txt")); err == nil {
		t.Error("файл не удалён")
	}
	if !strings.Contains(out, "- old/f.txt") || !strings.Contains(out, "nope.txt") {
		t.Errorf("вывод:\n%s", out)
	}
}

func TestApplyStopsOnRejectedPatch(t *testing.T) {
	dir, inbox := project(t)
	first := filepath.Join(inbox, "patch-1.zip")
	second := filepath.Join(inbox, "patch-2.zip")
	third := filepath.Join(inbox, "patch-3.zip")
	zipPatch(t, first, map[string]string{"one.txt": "1"})
	zipPatch(t, second, map[string]string{".sky/sky.conf": "scope = [evil/]"})
	zipPatch(t, third, map[string]string{"three.txt": "3"})
	now := time.Now()
	for i, p := range []string{first, second, third} {
		os.Chtimes(p, now.Add(time.Duration(i)*time.Minute), now.Add(time.Duration(i)*time.Minute))
	}
	code, _, errs := run(New(), dir, "apply")
	if code != 1 || !strings.Contains(errs, "patch-2.zip") {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "one.txt")); err != nil {
		t.Error("первый патч уже применён и остаётся")
	}
	if _, err := os.Stat(filepath.Join(dir, "three.txt")); err == nil {
		t.Error("после отказа следующие патчи не применяются")
	}
	for _, p := range []string{second, third} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s должен остаться в inbox", filepath.Base(p))
		}
	}
	if b, _ := os.ReadFile(repo.At(dir).Conf); strings.Contains(string(b), "evil") {
		t.Error("sky.conf изменён патчем")
	}
}

func TestApplyRequiresInit(t *testing.T) {
	if code, _, errs := run(New(), gitDir(t), "apply"); code != 1 || !strings.Contains(errs, "sky init") {
		t.Fatalf("%d %s", code, errs)
	}
}

func TestReadmeDescribesApplyAndOperations(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	b, _ := os.ReadFile(filepath.Join(repo.At(dir).Readme, "readme.md"))
	for _, want := range []string{"`sky apply [--force]`", "## Операции", "- `delete` — поле: `путь`", "## Патчи"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("в readme.md нет %q", want)
		}
	}
}

func TestApplyCommitsEachPatchInOrder(t *testing.T) {
	dir, inbox := project(t)
	zipPatch(t, filepath.Join(inbox, "patch-1.zip"), map[string]string{
		"project/a.txt": "a\n", ".sky/operation/commit": "feat: add a\n",
	})
	zipPatch(t, filepath.Join(inbox, "patch-2.zip"), map[string]string{
		"project/b.txt": "b\n", ".sky/operation/commit": "fix: add b\n",
	})
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(inbox, "patch-1.zip"), old, old)
	code, out, errs := run(New(), dir, "apply")
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if n := strings.Count(out, "● коммит "); n != 2 {
		t.Errorf("строк «● коммит»: %d\n%s", n, out)
	}
	if log := git(t, dir, "log", "--format=%s", "-2"); log != "fix: add b\nfeat: add a" {
		t.Errorf("история: %q", log)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}
