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
