package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/repo"
)

func withConf(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(repo.At(dir).Conf, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTestAndBuildRunInScopeDir(t *testing.T) {
	dir := gitDir(t)
	run(scopedApp(), dir, "init", "backend/", "frontend/")
	withConf(t, dir, "scope = [backend/, frontend/]\n\n[backend/]\ntest = pwd > ran.txt; echo T\nbuild = echo B > built.txt\n\n[frontend/]\ntest = echo F\n")

	code, out, errs := run(scopedApp(), dir, "test")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	for _, want := range []string{"── backend/ · test", "── frontend/ · test", "T", "F"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе нет %q:\n%s", want, out)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "backend", "ran.txt"))
	if got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(b))); got != mustReal(t, filepath.Join(dir, "backend")) {
		t.Errorf("команда должна идти из папки scope: %q", b)
	}

	code, out, _ = run(scopedApp(), dir, "build")
	if code != 0 || !strings.Contains(out, "команда build не задана") {
		t.Fatalf("build: %d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend", "built.txt")); err != nil {
		t.Error("build backend не выполнен")
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScopedSelectAndErrors(t *testing.T) {
	dir := gitDir(t)
	run(scopedApp(), dir, "init", "a/", "b/")
	withConf(t, dir, "scope = [a/, b/]\n\n[a/]\ntest = echo A\n\n[b/]\ntest = echo B\n")

	code, out, _ := run(scopedApp(), dir, "test", "b")
	if code != 0 || strings.Contains(out, "A") || !strings.Contains(out, "── b/ · test") {
		t.Fatalf("выбор папки: %d %s", code, out)
	}
	if code, _, errs := run(scopedApp(), dir, "test", "zzz/"); code != 1 || !strings.Contains(errs, "нет в scope") {
		t.Errorf("папка вне scope: %d %s", code, errs)
	}
}

func TestScopedFailureAggregates(t *testing.T) {
	dir := gitDir(t)
	run(scopedApp(), dir, "init", "a/", "b/")
	withConf(t, dir, "scope = [a/, b/]\n\n[a/]\ntest = exit 3\n\n[b/]\ntest = echo ok > done.txt\n")
	code, _, errs := run(scopedApp(), dir, "test")
	if code != 1 || !strings.Contains(errs, "a/") {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "b", "done.txt")); err != nil {
		t.Error("после падения a/ остальные папки всё равно запускаются")
	}
}

func TestScopedNothingConfigured(t *testing.T) {
	dir := gitDir(t)
	run(scopedApp(), dir, "init")
	if code, _, errs := run(scopedApp(), dir, "test"); code != 1 || !strings.Contains(errs, "не задана ни в одной папке") {
		t.Fatalf("%d %s", code, errs)
	}
}

func TestScopedMissingDir(t *testing.T) {
	dir := gitDir(t)
	run(scopedApp(), dir, "init")
	os.Remove(filepath.Join(dir, "project"))
	withConf(t, dir, "scope = [project/]\n\n[project/]\ntest = echo x\n")
	if code, _, errs := run(scopedApp(), dir, "test"); code != 1 || !strings.Contains(errs, "папки нет") {
		t.Fatalf("%d %s", code, errs)
	}
}

// scopedApp — приложение с запущенными вручную test/build: в реестр sky они пока не входят,
// имя build зарезервировано за конвейером «сборка + заморозка».
func scopedApp() *App {
	return &App{Commands: []Command{initCommand(), testCommand(), buildCommand()}}
}
