package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func gitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git недоступен: %v %s", err, out)
	}
	return dir
}

func run(a *App, dir string, args ...string) (int, string, string) {
	var o, e bytes.Buffer
	code := a.Run(dir, args, &o, &e)
	return code, o.String(), e.String()
}

func TestInitDefault(t *testing.T) {
	dir := gitDir(t)
	code, out, errs := run(New(), dir, "init")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	l := repo.At(dir)
	for _, d := range []string{l.Stages, l.Clouds} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("нет папки %s", d)
		}
	}
	b, _ := os.ReadFile(l.Conf)
	c, err := conf.Parse(string(b))
	if err != nil || len(c.Scope) != 1 || c.Scope[0] != "project/" {
		t.Fatalf("sky.conf: %q %v", b, err)
	}
	for _, name := range []string{"readme.md", "git-versioning.md"} {
		if fi, err := os.Stat(filepath.Join(l.Readme, name)); err != nil || fi.Mode().Perm() != 0o444 {
			t.Fatalf("%s: %v %v", name, fi, err)
		}
	}
	if st, err := os.Stat(l.Foryou); err != nil || !st.IsDir() {
		t.Fatal("нет foryou/")
	}
	if es, _ := os.ReadDir(l.Clouds); len(es) != 0 {
		t.Fatalf("clouds/ должна быть пустой: %v", es)
	}
	if g, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); !strings.Contains(string(g), ".sky/") || !strings.Contains(string(g), "*.db") {
		t.Fatalf(".gitignore должен быть шаблоном с .sky/: %q", g)
	}
	if st, err := os.Stat(filepath.Join(dir, "project")); err != nil || !st.IsDir() {
		t.Fatalf("папка project/ должна быть создана: %s", out)
	}
	if !strings.Contains(out, "✓ project/ создана") {
		t.Fatalf("вывод: %s", out)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "README.md")); err != nil || !strings.Contains(string(b), "- `project/`") {
		t.Fatalf("README.md: %q %v", b, err)
	}
}

func TestInitScopeForms(t *testing.T) {
	for _, args := range [][]string{
		{"init", "backend/", "frontend/"},
		{"init", "[backend/, frontend/]"},
	} {
		dir := gitDir(t)
		if code, _, errs := run(New(), dir, args...); code != 0 {
			t.Fatalf("%v: %s", args, errs)
		}
		b, _ := os.ReadFile(repo.At(dir).Conf)
		c, _ := conf.Parse(string(b))
		if len(c.Scope) != 2 || c.Scope[0] != "backend/" || c.Scope[1] != "frontend/" {
			t.Fatalf("%v: %v", args, c.Scope)
		}
	}
}

func TestInitIdempotentKeepsConf(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init", "backend/", "frontend/")
	l := repo.At(dir)
	custom := "scope = [backend/, frontend/]\ninbox = /tmp/in\nregulation = https://example.com/r.md\n"
	os.WriteFile(l.Conf, []byte(custom), 0o644)

	code, out, _ := run(New(), dir, "init")
	if code != 0 || !strings.Contains(out, "sky.conf не тронут") || !strings.Contains(out, "readme.md актуален") {
		t.Fatalf("%d %s", code, out)
	}
	if b, _ := os.ReadFile(l.Conf); string(b) != custom {
		t.Fatalf("конфиг изменён: %q", b)
	}

	run(New(), dir, "init", "app/")
	b, _ := os.ReadFile(l.Conf)
	c, _ := conf.Parse(string(b))
	if len(c.Scope) != 1 || c.Scope[0] != "app/" || c.Inbox != "/tmp/in" || c.Regulation != "https://example.com/r.md" {
		t.Fatalf("scope должен смениться, остальное остаться: %v", c)
	}
}

func TestInitRestoresReadme(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	l := repo.At(dir)
	p := filepath.Join(l.Readme, "readme.md")
	orig, _ := os.ReadFile(p)
	os.Chmod(p, 0o644)
	os.WriteFile(p, []byte("чужая правка"), 0o644)
	os.WriteFile(filepath.Join(l.Readme, "лишний.md"), []byte("x"), 0o644)
	_, out, _ := run(New(), dir, "init")
	if !strings.Contains(out, "перезаписан") || !strings.Contains(out, "удалён") {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(p); string(b) != string(orig) {
		t.Fatal("readme.md не восстановлен")
	}
}

func TestInitMigratesLegacyReglament(t *testing.T) {
	dir := gitDir(t)
	l := repo.At(dir)
	os.MkdirAll(l.Clouds, 0o755)
	old := filepath.Join(l.Clouds, "sky-reglament.md")
	os.WriteFile(old, []byte("старая записка"), 0o444)
	if code, _, errs := run(New(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatal("старая записка в clouds/ должна быть удалена")
	}
}

func TestInitCreatesGitRepoOutsideGit(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := run(New(), dir, "init")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if !strings.Contains(out, "git init") {
		t.Errorf("вывод: %s", out)
	}
	b, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(b)) != "dev" {
		t.Fatalf("ветка: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sky", "sky.conf")); err != nil {
		t.Error("нет .sky/sky.conf")
	}
	if code, out, _ := run(New(), dir, "init"); code != 0 || strings.Contains(out, "git init") {
		t.Errorf("повторный init не должен создавать репозиторий: %s", out)
	}
}

func TestInitKeepsExistingFiles(t *testing.T) {
	dir := gitDir(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("мой ридми"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin"), 0o644)
	if code, _, errs := run(New(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "мой ридми" {
		t.Errorf("README.md перезаписан: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(b) != "bin\n.sky/\n" {
		t.Errorf(".gitignore: %q", b)
	}
}

func TestInitCreatesScopeDirs(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init", "backend/", "frontend/")
	for _, d := range []string{"backend", "frontend"} {
		if st, err := os.Stat(filepath.Join(dir, d)); err != nil || !st.IsDir() {
			t.Errorf("папка %s не создана", d)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "project")); err == nil {
		t.Error("project/ не должна создаваться при явном scope")
	}
	dir2 := gitDir(t)
	if code, _, errs := run(New(), dir2, "init", "./"); code != 0 {
		t.Fatal(errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir2, "README.md")); !strings.Contains(string(b), "`./`") {
		t.Errorf("README для корневого scope: %s", b)
	}
}

func TestInitErrors(t *testing.T) {
	if code, _, _ := run(New(), gitDir(t), "init", "../x"); code != 1 {
		t.Fatal("scope вне репозитория должен падать")
	}
}

func TestInitDropsCommandsOfRemovedScope(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init", "a/", "b/")
	l := repo.At(dir)
	os.WriteFile(l.Conf, []byte("scope = [a/, b/]\n\n[a/]\ntest = echo a\n\n[b/]\ntest = echo b\n"), 0o644)
	_, out, _ := run(New(), dir, "init", "a/")
	if !strings.Contains(out, "[b/]") {
		t.Errorf("нужно предупреждение о [b/]: %s", out)
	}
	b, _ := os.ReadFile(l.Conf)
	c, err := conf.Parse(string(b))
	if err != nil || c.Cmd["a/"].Test != "echo a" || len(c.Cmd) != 1 {
		t.Fatalf("команды: %v %v", c.Cmd, err)
	}
}

func TestReadmeStructure(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	l := repo.At(dir)
	b, _ := os.ReadFile(filepath.Join(l.Readme, "readme.md"))
	readme := string(b)
	for _, want := range []string{"sky init", "sky help", "scope = [project/]"} {
		if !strings.Contains(readme, want) {
			t.Errorf("в readme.md нет %q", want)
		}
	}
	m := regexp.MustCompile(`\]\(([^)#]+\.md)\)`).FindAllStringSubmatch(readme, -1)
	if len(m) == 0 {
		t.Fatal("в readme.md нет ссылок на соседние файлы")
	}
	for _, x := range m {
		if _, err := os.Stat(filepath.Join(l.Readme, x[1])); err != nil {
			t.Errorf("ссылка на несуществующий файл %s", x[1])
		}
	}
}

func TestNeedsInitHookHealsAndRequiresInit(t *testing.T) {
	a := New()
	called := 0
	a.Commands = append(a.Commands, Command{Name: "probe", Usage: "sky probe", Summary: "тест", NeedsInit: true,
		Run: func(*App, Env, []string) error { called++; return nil }})
	dir := gitDir(t)
	if code, _, errs := run(a, dir, "probe"); code != 1 || !strings.Contains(errs, "sky init") || called != 0 {
		t.Fatalf("без init: %d %s", code, errs)
	}
	run(a, dir, "init")
	l := repo.At(dir)
	p := filepath.Join(l.Readme, "readme.md")
	os.Remove(p)
	if code, _, errs := run(a, dir, "probe"); code != 0 || called != 1 {
		t.Fatalf("%d %s", code, errs)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "sky probe") {
		t.Fatal("readme.md не восстановлен с новой командой")
	}
}

func TestHelpVersionUnknown(t *testing.T) {
	a := New()
	if code, out, _ := run(a, t.TempDir(), "help"); code != 0 || !strings.Contains(out, "sky init") {
		t.Fatal(out)
	}
	if code, out, _ := run(a, t.TempDir(), "--version"); code != 0 || !strings.HasPrefix(out, "sky ") {
		t.Fatal(out)
	}
	if code, _, _ := run(a, t.TempDir()); code != 2 {
		t.Fatal("без аргументов — код 2")
	}
	if code, _, _ := run(a, t.TempDir(), "nope"); code != 2 {
		t.Fatal("неизвестная команда — код 2")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestInitFirstCommitAndDev(t *testing.T) {
	dir := t.TempDir()
	if code, out, errs := run(New(), dir, "init"); code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if b := git(t, dir, "branch", "--show-current"); b != "dev" {
		t.Fatalf("активная ветка: %q", b)
	}
	if bs := git(t, dir, "branch", "--format=%(refname:short)"); bs != "dev\nmain" {
		t.Fatalf("ветки: %q", bs)
	}
	if n := git(t, dir, "rev-list", "--count", "main"); n != "1" {
		t.Fatalf("коммитов в main: %s", n)
	}
	if git(t, dir, "rev-parse", "main") != git(t, dir, "rev-parse", "dev") {
		t.Error("dev должна стартовать с первого коммита main")
	}
	if m := git(t, dir, "log", "-1", "--format=%s", "main"); !strings.Contains(m, "At the moment") {
		t.Errorf("сообщение: %q", m)
	}
	if f := git(t, dir, "ls-tree", "-r", "--name-only", "main"); f != ".gitignore\nREADME.md" {
		t.Errorf("в коммите: %q", f)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}

func TestInitLeavesExistingHistoryAlone(t *testing.T) {
	dir := gitDir(t)
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "old")
	before := git(t, dir, "rev-parse", "HEAD")
	branch := git(t, dir, "branch", "--show-current")
	run(New(), dir, "init")
	if git(t, dir, "rev-parse", "HEAD") != before || git(t, dir, "branch", "--show-current") != branch {
		t.Error("init не должен трогать репозиторий с историей")
	}
	if git(t, dir, "branch", "--format=%(refname:short)") != branch {
		t.Error("лишние ветки в репозитории с историей")
	}
}

func TestInitHelp(t *testing.T) {
	for _, args := range [][]string{{"init", "--help"}, {"init", "-h"}, {"help", "init"}} {
		code, out, errs := run(New(), t.TempDir(), args...)
		if code != 0 || errs != "" {
			t.Fatalf("%v: %d %s", args, code, errs)
		}
		for _, want := range []string{"sky init — справка", "Задача Claude", "── Синтаксис", "sky init ./"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: нет %q", args, want)
			}
		}
	}
}

func TestInitHelpDoesNotInit(t *testing.T) {
	dir := t.TempDir()
	run(New(), dir, "init", "--help")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("--help не должен создавать репозиторий")
	}
	if _, err := os.Stat(filepath.Join(dir, ".sky")); err == nil {
		t.Error("--help не должен создавать .sky")
	}
}

func TestHelpUnknownAndPlain(t *testing.T) {
	if code, _, _ := run(New(), t.TempDir(), "help", "nope"); code != 2 {
		t.Errorf("help nope: %d", code)
	}
	if code, out, _ := run(New(), t.TempDir(), "apply", "--help"); code != 0 || !strings.Contains(out, "sky apply") {
		t.Errorf("apply --help: %d %s", code, out)
	}
}

func TestInitHelpIsNeutral(t *testing.T) {
	for _, bad := range []string{"clc.sh", "Док", "Ζωή", "zoe", "Makefile"} {
		if strings.Contains(initHelp, bad) {
			t.Errorf("справка содержит %q", bad)
		}
	}
}

func TestInitCreatesLicense(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	l := repo.At(dir)
	b, _ := os.ReadFile(l.Conf)
	os.WriteFile(l.Conf, append(b, []byte("\n[license]\ntype = MIT\nholder = Acme Ltd\nyear = 2030\n")...), 0o644)
	code, out, errs := run(New(), dir, "init")
	if code != 0 || !strings.Contains(out, "✓ LICENSE создан (MIT)") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	lic, _ := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if !strings.Contains(string(lic), "Copyright (c) 2030 Acme Ltd") {
		t.Errorf("LICENSE: %.80q", lic)
	}
	os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("мой"), 0o644)
	run(New(), dir, "init")
	if b, _ := os.ReadFile(filepath.Join(dir, "LICENSE")); string(b) != "мой" {
		t.Error("существующий LICENSE перезаписан")
	}
}

func TestInitLicenseUrlCreatesNoFile(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	b, _ := os.ReadFile(repo.At(dir).Conf)
	os.WriteFile(repo.At(dir).Conf, append(b, []byte("\n[license]\nurl = https://e.org/l\n")...), 0o644)
	if code, _, errs := run(New(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err == nil {
		t.Error("для url файл создаваться не должен")
	}
}

func TestInitLicenseInFirstCommit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".sky"), 0o755)
	exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run()
	os.WriteFile(repo.At(dir).Conf, []byte("scope = [project/]\n\n[license]\ntype = Unlicense\n"), 0o644)
	if code, _, errs := run(New(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	if f := git(t, dir, "ls-tree", "-r", "--name-only", "main"); f != ".gitignore\nLICENSE\nREADME.md" {
		t.Errorf("в первом коммите: %q", f)
	}
	if r, _ := os.ReadFile(filepath.Join(dir, "README.md")); !strings.Contains(string(r), "Unlicense, см. [LICENSE]") {
		t.Errorf("README: %s", r)
	}
}

func TestInitBrokenLicenseStopsBeforeWriting(t *testing.T) {
	dir := gitDir(t)
	run(New(), dir, "init")
	b, _ := os.ReadFile(repo.At(dir).Conf)
	os.WriteFile(repo.At(dir).Conf, append(b, []byte("\n[license]\ntype = MIT\n")...), 0o644)
	if code, _, errs := run(New(), dir, "init"); code != 1 || !strings.Contains(errs, "holder") {
		t.Fatalf("%d %s", code, errs)
	}
}
