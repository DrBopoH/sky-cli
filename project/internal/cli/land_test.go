package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestLandSeedThroughConsole(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	gitIn(t, dir, "checkout", "-q", "-b", "feature")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("f\n"), 0o644)
	gitIn(t, dir, "add", "f.txt")
	gitIn(t, dir, "commit", "-q", "-m", "feat: f")

	code, out, errs := run(newApp(), dir, "land", "Add f", "--seed", "--dry")
	if code != 0 || !strings.Contains(out, "--dry") || gitIn(t, dir, "symbolic-ref", "--short", "HEAD") != "feature" {
		t.Fatalf("dry: %d %s %s", code, out, errs)
	}
	code, out, errs = run(newApp(), dir, "land", "Add f", "--seed", "--from", "1.1.35+35")
	if code != 0 || !strings.Contains(out, "✓ Add f | v1.1.36+36.") {
		t.Fatalf("land: %d %s %s", code, out, errs)
	}
	if got := gitIn(t, dir, "log", "-1", "--format=%s", "dev"); !strings.HasPrefix(got, "Add f | v1.1.36+36.") {
		t.Fatalf("dev: %s", got)
	}
}

func TestLandArgumentErrors(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	for _, args := range [][]string{
		{"land"},
		{"land", "a", "b"},
		{"land", "x", "--nope"},
		{"land", "x", "--module"},
	} {
		if code, _, errs := run(newApp(), dir, args...); code != 1 || errs == "" {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
}

// landWithOrigin — репозиторий с bare-origin и веткой feature, готовой к land --seed.
func landWithOrigin(t *testing.T) (dir, bare string) {
	t.Helper()
	dir = t.TempDir()
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	bare = filepath.Join(t.TempDir(), "o.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	gitIn(t, dir, "remote", "add", "origin", bare)
	gitIn(t, dir, "checkout", "-q", "-b", "feature")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("f\n"), 0o644)
	gitIn(t, dir, "add", "f.txt")
	gitIn(t, dir, "commit", "-q", "-m", "feat: f")
	return dir, bare
}

func remoteTip(bare, branch string) string {
	out, _ := exec.Command("git", "-C", bare, "rev-parse", "--verify", "-q", "refs/heads/"+branch).Output()
	return strings.TrimSpace(string(out))
}

func TestLandPushesSharedBranchToOrigin(t *testing.T) {
	dir, bare := landWithOrigin(t)
	code, out, errs := run(newApp(), dir, "land", "Add f", "--seed", "--from", "1.1.35+35")
	if code != 0 || !strings.Contains(out, "push origin dev") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if remoteTip(bare, "dev") != gitIn(t, dir, "rev-parse", "dev") {
		t.Error("dev должна оказаться в origin")
	}
	if remoteTip(bare, "feature") != "" {
		t.Error("ветка фичи (не общая) в origin не уходит")
	}
}

func TestLandLocalFlagDoesNotPush(t *testing.T) {
	dir, bare := landWithOrigin(t)
	code, out, errs := run(newApp(), dir, "land", "Add f", "--seed", "--from", "1.1.35+35", "--local")
	if code != 0 || strings.Contains(out, "push origin") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if remoteTip(bare, "dev") != "" {
		t.Error("с --local ничего не уходит")
	}
}

func TestLandPushFailureKeepsLocalResult(t *testing.T) {
	dir, bare := landWithOrigin(t)
	os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	code, out, errs := run(newApp(), dir, "land", "Add f", "--seed", "--from", "1.1.35+35")
	if code != 0 || !strings.Contains(out, "push не удался") || !strings.Contains(out, "✓ Add f | v1.1.36+36.") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if !strings.HasPrefix(gitIn(t, dir, "log", "-1", "--format=%s", "dev"), "Add f | v1.1.36+36.") {
		t.Error("локальный результат должен остаться")
	}
}
