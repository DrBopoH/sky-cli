package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildRepo(t *testing.T, confLine string) string {
	t.Helper()
	dir := t.TempDir()
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	if confLine != "" {
		p := filepath.Join(dir, ".sky", "sky.conf")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, append(b, []byte("\n"+confLine+"\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("f\n"), 0o644)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "Add f | v1.1.1+1.abc1234")
	return dir
}

func TestBuildThroughConsole(t *testing.T) {
	dir := buildRepo(t, "build = ./ echo compiled")
	code, out, errs := run(newApp(), dir, "build", "--dry")
	if code != 0 || !strings.Contains(out, "--dry") || strings.Contains(out, "compiled\n$") {
		t.Fatalf("dry: %d %s %s", code, out, errs)
	}
	if gitIn(t, dir, "branch", "--list", "build/*") != "" {
		t.Fatal("dry создал ветку")
	}
	code, out, errs = run(newApp(), dir, "build", "b", "first beta")
	if code != 0 || !strings.Contains(out, "compiled") || !strings.Contains(out, "✓ Build 1 beta: first beta | v1.b1.1+1.") {
		t.Fatalf("build: %d %s %s", code, out, errs)
	}
	br := strings.TrimSpace(strings.TrimPrefix(gitIn(t, dir, "branch", "--list", "build/*"), "*"))
	if !strings.HasPrefix(br, "build/") || gitIn(t, dir, "symbolic-ref", "--short", "HEAD") != "dev" {
		t.Fatalf("ветка %q", br)
	}
	md, err := os.ReadFile(filepath.Join(dir, ".sky", "foryou", "build.md"))
	if err != nil || !strings.Contains(string(md), "**создана**") {
		t.Fatalf("build.md: %v %s", err, md)
	}
	// look после других команд не стирает build.md
	if code, _, errs := run(newApp(), dir, "look"); code != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sky", "foryou", "build.md")); err != nil {
		t.Error("look стёр build.md")
	}
}

func TestBuildFailureWritesReportNoBranch(t *testing.T) {
	dir := buildRepo(t, "build = ./ echo boom; exit 2")
	code, _, errs := run(newApp(), dir, "build")
	if code != 1 || !strings.Contains(errs, "код 2") {
		t.Fatalf("%d %s", code, errs)
	}
	if gitIn(t, dir, "branch", "--list", "build/*") != "" {
		t.Fatal("ветка после провала")
	}
	md, _ := os.ReadFile(filepath.Join(dir, ".sky", "foryou", "build.md"))
	if !strings.Contains(string(md), "boom") {
		t.Fatalf("build.md: %s", md)
	}
}

func TestBuildNoPointsExplains(t *testing.T) {
	dir := buildRepo(t, "")
	code, out, errs := run(newApp(), dir, "build")
	if code != 0 || !strings.Contains(out, "запуск сборки пропущен") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
}

func TestBuildArgumentErrors(t *testing.T) {
	dir := buildRepo(t, "")
	for _, args := range [][]string{{"build", "--nope"}, {"build", "a", "x", "y"}, {"build", "--from"}} {
		if code, _, errs := run(newApp(), dir, args...); code != 1 || errs == "" {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
}

func TestFixThroughConsole(t *testing.T) {
	dir := buildRepo(t, "")
	if code, _, errs := run(newApp(), dir, "build"); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	br := strings.TrimSpace(strings.TrimPrefix(gitIn(t, dir, "branch", "--list", "build/*"), "*"))
	gitIn(t, dir, "checkout", "-q", "-b", "hot", br)
	os.WriteFile(filepath.Join(dir, "h.txt"), []byte("h\n"), 0o644)
	gitIn(t, dir, "add", "h.txt")
	gitIn(t, dir, "commit", "-q", "-m", "fix: h")
	code, out, errs := run(newApp(), dir, "fix", "Fix h", "--dry")
	if code != 0 || !strings.Contains(out, "--dry") || gitIn(t, dir, "symbolic-ref", "--short", "HEAD") != "hot" {
		t.Fatalf("dry: %d %s %s", code, out, errs)
	}
	code, out, errs = run(newApp(), dir, "fix", "Fix h")
	if code != 0 || !strings.Contains(out, "✓ Fix h | v1.a1-1.1+2.") {
		t.Fatalf("fix: %d %s %s", code, out, errs)
	}
	if got := gitIn(t, dir, "log", "-1", "--format=%s", br); !strings.HasPrefix(got, "Fix h | v1.a1-1.1+2.") {
		t.Fatalf("ветка сборки: %s", got)
	}
	if code, _, errs := run(newApp(), dir, "fix"); code != 1 || errs == "" {
		t.Errorf("без резюме: %d %q", code, errs)
	}
}
