package cli

import (
	"os"
	"os/exec"
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

func TestFeatureRemembersTargetForLandAndFix(t *testing.T) {
	dir := buildRepo(t, "")
	commitFile := func(name string) {
		os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644)
		gitIn(t, dir, "add", name)
		gitIn(t, dir, "commit", "-q", "-m", "feat: "+name)
	}
	if code, _, errs := run(newApp(), dir, "feature", "Bad Name"); code != 1 || errs == "" {
		t.Fatalf("плохое имя: %d %q", code, errs)
	}
	code, out, errs := run(newApp(), dir, "feature", "add-x", "--seed")
	if code != 0 || !strings.Contains(out, "wip/") {
		t.Fatalf("feature: %d %s %s", code, out, errs)
	}
	if b := gitIn(t, dir, "symbolic-ref", "--short", "HEAD"); !strings.HasPrefix(b, "wip/") || !strings.HasSuffix(b, "/add-x") {
		t.Fatalf("ветка: %s", b)
	}
	commitFile("x.txt")
	// land без флагов берёт цель из ветки
	code, out, errs = run(newApp(), dir, "land", "Add x")
	if code != 0 || !strings.Contains(out, "✓ Add x | v1.1.2+2.") {
		t.Fatalf("land: %d %s %s", code, out, errs)
	}
	if code, _, errs := run(newApp(), dir, "build"); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	code, out, errs = run(newApp(), dir, "feature", "hot", "--fix")
	if code != 0 || !strings.Contains(out, "build/") {
		t.Fatalf("feature --fix: %d %s %s", code, out, errs)
	}
	commitFile("h.txt")
	code, out, errs = run(newApp(), dir, "land", "Fix hot")
	if code != 0 || !strings.Contains(out, "✓ Fix hot | v1.a1-1.2+3.") {
		t.Fatalf("land на ветке исправления: %d %s %s", code, out, errs)
	}
	if b := gitIn(t, dir, "symbolic-ref", "--short", "HEAD"); !strings.HasPrefix(b, "build/") {
		t.Fatalf("HEAD после исправления: %s", b)
	}
}

func TestModulesAndPublishThroughConsole(t *testing.T) {
	dir := buildRepo(t, "")
	reg := filepath.Join(dir, "docs", "PROJECT-GRAPH.md")
	os.MkdirAll(filepath.Dir(reg), 0o755)
	os.WriteFile(reg, []byte("| Модуль | Зависит от |\n|---|---|\n| leaf | — |\n| main | leaf |\n"), 0o644)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "Add graph | v1.1.2+2.abc1234")
	code, out, errs := run(newApp(), dir, "modules")
	if code != 0 || !strings.Contains(out, "зерно: main") || !strings.Contains(out, "leaf") {
		t.Fatalf("modules: %d %s %s", code, out, errs)
	}
	if code, out, _ := run(newApp(), dir, "modules", "leaf"); code != 0 || !strings.Contains(out, "dev → leaf") {
		t.Fatalf("modules leaf: %d %s", code, out)
	}
	if code, _, _ := run(newApp(), dir, "modules", "a", "b"); code == 0 {
		t.Fatal("два модуля — ошибка")
	}
	if code, _, _ := run(newApp(), dir, "publish"); code == 0 {
		t.Fatal("нет main и сборок — ошибка")
	}
	if code, _, errs := run(newApp(), dir, "build"); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	code, out, errs = run(newApp(), dir, "publish", "--dry")
	if code != 0 || !strings.Contains(out, "--dry") || !strings.Contains(out, "Build 1 alpha | v1.a1.2+2.") {
		t.Fatalf("publish --dry: %d %s %s", code, out, errs)
	}
	code, _, errs = run(newApp(), dir, "publish")
	if code != 0 {
		t.Fatalf("publish: %s", errs)
	}
	if got := gitIn(t, dir, "log", "-1", "--format=%s", "main"); !strings.HasPrefix(got, "Build 1 alpha | v1.a1.2+2.") {
		t.Errorf("main: %q", got)
	}
}

func TestPrNeedsFeatureBranch(t *testing.T) {
	dir := buildRepo(t, "")
	code, _, errs := run(newApp(), dir, "pr", "Add f")
	if code == 0 || !strings.Contains(errs, "wip/") {
		t.Fatalf("на dev pr не работает: %d %s", code, errs)
	}
	if code, _, _ := run(newApp(), dir, "pr"); code == 0 {
		t.Fatal("нужно резюме")
	}
	gitIn(t, dir, "switch", "-q", "-c", "wip/1125-1005/x")
	if code, _, errs := run(newApp(), dir, "pr", "Add f"); code == 0 || !strings.Contains(errs, "sky feature") {
		t.Fatalf("ветка не заведена sky feature: %d %s", code, errs)
	}
}

func TestSyncNeedsRemote(t *testing.T) {
	dir := buildRepo(t, "")
	if code, _, errs := run(newApp(), dir, "sync"); code == 0 || !strings.Contains(errs, "нет удалённого") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, _ := run(newApp(), dir, "sync", "x"); code == 0 {
		t.Fatal("лишний аргумент")
	}
}

func TestMergeArgs(t *testing.T) {
	dir := buildRepo(t, "")
	for _, args := range [][]string{{"merge"}, {"merge", "1", "2"}, {"merge", "--bad", "1"}} {
		if code, _, _ := run(newApp(), dir, args...); code == 0 {
			t.Errorf("%v должна отказать", args)
		}
	}
	if code, _, errs := run(newApp(), dir, "merge", "7"); code == 0 || !strings.Contains(errs, "нет удалённого") {
		t.Fatalf("без origin: %d %s", code, errs)
	}
}

func TestProtectArgs(t *testing.T) {
	dir := buildRepo(t, "")
	if code, _, _ := run(newApp(), dir, "protect", "x"); code == 0 {
		t.Fatal("лишний аргумент")
	}
	if code, _, _ := run(newApp(), dir, "protect", "--bad"); code == 0 {
		t.Fatal("неизвестный флаг")
	}
}

func TestPublishPushesMainToOrigin(t *testing.T) {
	for _, local := range []bool{false, true} {
		dir := buildRepo(t, "")
		bare := filepath.Join(t.TempDir(), "o.git")
		if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		gitIn(t, dir, "remote", "add", "origin", bare)
		if code, _, errs := run(newApp(), dir, "build"); code != 0 {
			t.Fatalf("build: %s", errs)
		}
		args := []string{"publish"}
		if local {
			args = append(args, "--local")
		}
		code, out, errs := run(newApp(), dir, args...)
		if code != 0 {
			t.Fatalf("publish: %d %s %s", code, out, errs)
		}
		pushed := remoteTip(bare, "main") != ""
		if pushed == local || (pushed && remoteTip(bare, "main") != gitIn(t, dir, "rev-parse", "main")) {
			t.Errorf("local=%v pushed=%v: main в origin должна быть, только если не --local", local, pushed)
		}
	}
}
