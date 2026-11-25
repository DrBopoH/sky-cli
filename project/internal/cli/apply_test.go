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
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
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
	code, _, errs := run(newApp(), dir, "apply")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "project", "a.txt")); string(b) != "a" {
		t.Error("файл не записан")
	}
	if _, err := os.Stat(filepath.Join(inbox, "patch-one.zip")); err == nil {
		t.Error("патч должен уйти из inbox")
	}
	if code, _, errs := run(newApp(), dir, "apply"); code != 1 {
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
	if code, _, errs := run(newApp(), dir, "apply"); code != 0 {
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

	if code, _, errs := run(newApp(), dir, "apply"); code != 1 {
		t.Fatalf("грязное дерево должно останавливать: %d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err == nil {
		t.Fatal("ничего не должно быть записано")
	}
	if code, _, errs := run(newApp(), dir, "apply", "--force"); code != 0 {
		t.Fatalf("--force: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Error("с --force патч должен примениться")
	}
	if code, _, errs := run(newApp(), dir, "apply", "--nope"); code != 1 {
		t.Errorf("неизвестный аргумент: %d %s", code, errs)
	}
}

func TestApplyDeleteOperation(t *testing.T) {
	dir, inbox := project(t)
	os.MkdirAll(filepath.Join(dir, "old"), 0o755)
	os.WriteFile(filepath.Join(dir, "old", "f.txt"), []byte("x"), 0o644)
	commitAll(t, dir)
	zipPatch(t, filepath.Join(inbox, "patch.zip"), map[string]string{".sky/operation/delete": "old/f.txt\nnope.txt\n"})
	code, out, errs := run(newApp(), dir, "apply")
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
	code, _, errs := run(newApp(), dir, "apply")
	if code != 1 {
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
	if code, _, errs := run(newApp(), gitDir(t), "apply"); code != 1 {
		t.Fatalf("%d %s", code, errs)
	}
}

func TestReadmeListsOperations(t *testing.T) {
	dir := gitDir(t)
	run(newApp(), dir, "init")
	b, _ := os.ReadFile(filepath.Join(repo.At(dir).Readme, "readme.md"))
	app := newApp()
	if len(app.Operations) == 0 {
		t.Fatal("операции патча не переданы")
	}
	for _, o := range app.Operations { // readme берёт операции из реестра, не из копии текста
		if !strings.Contains(string(b), o.Usage) {
			t.Errorf("в readme.md нет операции %q", o.Usage)
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
	code, out, errs := run(newApp(), dir, "apply")
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

// landProject — project() с именованным коммитом в dev и реестром модулей leaf/main.
func landProject(t *testing.T) (dir, inbox string) {
	t.Helper()
	dir, inbox = project(t)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "PROJECT-GRAPH.md"), []byte("| Модуль | Зависит от |\n|---|---|\n| leaf | — |\n| main | leaf |\n"), 0o644)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "Add graph | v1.1.1+1.abc1234")
	return dir, inbox
}

func TestApplyLandSeedWithoutWip(t *testing.T) {
	dir, inbox := landProject(t)
	zipPatch(t, filepath.Join(inbox, "patch-a.zip"), map[string]string{"project/a.txt": "a", ".sky/operation/commit": "feat: add a\n"})
	code, out, errs := run(newApp(), dir, "apply", "--land", "Add a", "--seed")
	if code != 0 || !strings.Contains(out, "✓ Add a | v1.1.2+2.") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if gitIn(t, dir, "branch", "--show-current") != "dev" || gitIn(t, dir, "branch", "--list", "wip/*") != "" {
		t.Error("HEAD на dev, веток wip нет")
	}
	if !strings.HasPrefix(gitIn(t, dir, "log", "-1", "--format=%s", "dev"), "Add a | v1.1.2+2.") {
		t.Error("dev должна получить именованный коммит")
	}
	if _, err := os.Stat(filepath.Join(inbox, "patch-a.zip")); err == nil {
		t.Error("патч уходит из inbox")
	}
}

func TestApplyLandModuleTakesOldestAndStacks(t *testing.T) {
	dir, inbox := landProject(t)
	zipPatch(t, filepath.Join(inbox, "patch-1-first.zip"), map[string]string{"project/leaf/a.go": "a", ".sky/operation/commit": "feat: a\n"})
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(inbox, "patch-1-first.zip"), old, old)
	zipPatch(t, filepath.Join(inbox, "patch-2-second.zip"), map[string]string{"project/leaf/b.go": "b", ".sky/operation/commit": "feat: b\n"})
	code, out, errs := run(newApp(), dir, "apply", "--land", "Add leaf a", "--module", "leaf")
	if code != 0 || !strings.Contains(out, "в inbox ещё патчей: 1") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	br := gitIn(t, dir, "branch", "--show-current")
	if !strings.HasPrefix(br, "leaf/") || strings.HasPrefix(gitIn(t, dir, "log", "-1", "--format=%s", "dev"), "Add leaf") {
		t.Fatalf("слой leaf, dev не меняется: %s", br)
	}
	// второй патч идёт с ветки слоя: sky сам уходит на detached dev и вливает в ту же открытую ветку
	code, out, errs = run(newApp(), dir, "apply", "--land", "Add leaf b", "--module", "leaf")
	if code != 0 || gitIn(t, dir, "branch", "--show-current") != br || !strings.HasPrefix(gitIn(t, dir, "log", "-1", "--format=%s", br), "Add leaf b | v3.") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if gitIn(t, dir, "branch", "--list", "wip/*") != "" {
		t.Error("веток wip быть не должно")
	}
}

func TestApplyLandFailureLeavesWipForRepair(t *testing.T) {
	dir, inbox := landProject(t)
	zipPatch(t, filepath.Join(inbox, "patch-nocommit.zip"), map[string]string{"project/a.txt": "a"})
	code, _, errs := run(newApp(), dir, "apply", "--land", "Add a", "--seed")
	if code == 0 || !strings.Contains(errs, "wip/") {
		t.Fatalf("%d %s", code, errs)
	}
	if b := gitIn(t, dir, "branch", "--show-current"); !strings.HasPrefix(b, "wip/") || !strings.HasSuffix(b, "/nocommit") {
		t.Errorf("ветка разбора: %q", b)
	}
}

func TestApplyLandFlagErrors(t *testing.T) {
	dir, inbox := landProject(t)
	zipPatch(t, filepath.Join(inbox, "patch-x.zip"), map[string]string{"project/x.txt": "x", ".sky/operation/commit": "feat: x\n"})
	for _, args := range [][]string{
		{"apply", "--module", "leaf"},
		{"apply", "--land", "Add x"},
		{"apply", "--land", "Add x", "--seed", "--module", "leaf"},
		{"apply", "--land", "Add x", "--seed", "--force"},
	} {
		if code, _, _ := run(newApp(), dir, args...); code == 0 {
			t.Errorf("%v: ожидалась ошибка", args)
		}
	}
	if _, err := os.Stat(filepath.Join(inbox, "patch-x.zip")); err != nil {
		t.Error("при ошибке флагов патч остаётся в inbox")
	}
}
