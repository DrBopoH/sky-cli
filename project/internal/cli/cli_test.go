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
	code, out, errs := run(newApp(), dir, "init")
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
	for _, name := range []string{"readme.md", "docs.md", "MT-git-reglament.md", "MT-unify-systems.md", "MT-fix-before-reset.md"} {
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
	if g, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(g) != ".sky/\n" {
		t.Fatalf(".gitignore: только строка .sky/: %q", g)
	}
	for _, f := range []string{"README.md", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("init не создаёт %s", f)
		}
	}
	if st, err := os.Stat(filepath.Join(dir, "project")); err != nil || !st.IsDir() {
		t.Fatalf("папка project/ должна быть создана: %s", out)
	}
}

func TestInitScopeForms(t *testing.T) {
	for _, args := range [][]string{
		{"init", "backend/", "frontend/"},
		{"init", "[backend/, frontend/]"},
	} {
		dir := gitDir(t)
		if code, _, errs := run(newApp(), dir, args...); code != 0 {
			t.Fatalf("%v: %s", args, errs)
		}
		b, _ := os.ReadFile(repo.At(dir).Conf)
		c, _ := conf.Parse(string(b))
		if len(c.Scope) != 2 || c.Scope[0] != "backend/" || c.Scope[1] != "frontend/" {
			t.Fatalf("%v: %v", args, c.Scope)
		}
	}
}

func TestInitRunsOnce(t *testing.T) {
	dir := gitDir(t)
	run(newApp(), dir, "init", "backend/", "frontend/")
	l := repo.At(dir)
	custom := "scope = [backend/, frontend/]\ninbox = /tmp/in\n"
	os.WriteFile(l.Conf, []byte(custom), 0o644)
	code, _, errs := run(newApp(), dir, "init")
	if code != 1 {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, _ := run(newApp(), dir, "init", "app/"); code != 1 {
		t.Error("с путями тоже отказ")
	}
	if b, _ := os.ReadFile(l.Conf); string(b) != custom {
		t.Fatalf("конфиг изменён: %q", b)
	}
}

func TestInitCreatesGitRepoOutsideGit(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := run(newApp(), dir, "init")
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
	if code, out, _ := run(newApp(), dir, "init"); code != 1 || strings.Contains(out, "git init") {
		t.Errorf("повторный init отказывает и ничего не создаёт: %s", out)
	}
}

func TestInitKeepsExistingFiles(t *testing.T) {
	dir := gitDir(t)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("мой ридми"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin"), 0o644)
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "мой ридми" {
		t.Errorf("README.md перезаписан: %q", b)
	}
	if files := git(t, dir, "show", "--name-only", "--format=", "HEAD"); strings.TrimSpace(files) != ".gitignore" {
		t.Errorf("в первый коммит входит только .gitignore: %q", files)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(b) != "bin\n.sky/\n" {
		t.Errorf(".gitignore: %q", b)
	}
}

func TestInitCreatesScopeDirs(t *testing.T) {
	dir := gitDir(t)
	run(newApp(), dir, "init", "backend/", "frontend/")
	for _, d := range []string{"backend", "frontend"} {
		if st, err := os.Stat(filepath.Join(dir, d)); err != nil || !st.IsDir() {
			t.Errorf("папка %s не создана", d)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "project")); err == nil {
		t.Error("project/ не должна создаваться при явном scope")
	}
	dir2 := gitDir(t)
	if code, _, errs := run(newApp(), dir2, "init", "./"); code != 0 {
		t.Fatal(errs)
	}
}

func TestInitErrors(t *testing.T) {
	if code, _, _ := run(newApp(), gitDir(t), "init", "../x"); code != 1 {
		t.Fatal("scope вне репозитория должен падать")
	}
}

func TestReadmeStructure(t *testing.T) {
	dir := gitDir(t)
	run(newApp(), dir, "init")
	l := repo.At(dir)
	b, _ := os.ReadFile(filepath.Join(l.Readme, "readme.md"))
	readme := string(b)
	for _, e := range newApp().Entries() { // readme собирается из реестра: каждая команда в нём есть
		if !strings.Contains(readme, e.Usage) {
			t.Errorf("в readme.md нет команды %q", e.Usage)
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
	a := newApp()
	called := 0
	a.Commands = append(a.Commands, Command{Name: "probe", Usage: "sky probe", Summary: "тест", NeedsInit: true,
		Run: func(*App, Env, []string) error { called++; return nil }})
	dir := gitDir(t)
	if code, _, errs := run(a, dir, "probe"); code != 1 || called != 0 {
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
	a := newApp()
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

// initWithFiles — init и затем README.md и .gitignore (*.db), закоммиченные автором: init сам эти файлы не создаёт.
func initWithFiles(t *testing.T, dir string) {
	t.Helper()
	if code, _, errs := run(newApp(), dir, "init"); code != 0 {
		t.Fatal(errs)
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# t\n"), 0o644)
	f, _ := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("*.db\n")
	f.Close()
	git(t, dir, "add", "README.md", ".gitignore")
	git(t, dir, "commit", "-q", "-m", "chore: files")
}

func TestInitFirstCommitAndDev(t *testing.T) {
	dir := t.TempDir()
	if code, out, errs := run(newApp(), dir, "init"); code != 0 {
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
	if f := git(t, dir, "ls-tree", "-r", "--name-only", "main"); f != ".gitignore" {
		t.Errorf("в коммите: %q", f)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}

func historyRepo(t *testing.T) string {
	t.Helper()
	dir := gitDir(t)
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "old")
	return dir
}

func TestInitInHistoryCreatesDevFromMain(t *testing.T) {
	dir := historyRepo(t)
	before := git(t, dir, "rev-parse", "HEAD")
	main := git(t, dir, "branch", "--show-current")
	code, out, errs := run(newApp(), dir, "init")
	if code != 0 || !strings.Contains(out, "ветка dev создана от вершины "+main) {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if git(t, dir, "rev-parse", "HEAD") != before || git(t, dir, "rev-parse", main) != before || git(t, dir, "rev-parse", "dev") != before {
		t.Error("история не должна меняться: dev стартует с вершины main")
	}
	if git(t, dir, "branch", "--show-current") != "dev" {
		t.Error("работа продолжается на dev")
	}
}

func TestInitInHistoryOffMainOnlyHints(t *testing.T) {
	dir := historyRepo(t)
	git(t, dir, "switch", "-q", "-c", "topic")
	code, out, errs := run(newApp(), dir, "init")
	if code != 0 || !strings.Contains(out, "git switch -c dev") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if git(t, dir, "branch", "--show-current") != "topic" || strings.Contains(git(t, dir, "branch", "--format=%(refname:short)"), "dev") {
		t.Error("с чужой ветки dev не заводим")
	}
}

func TestInitInHistoryKeepsExistingDev(t *testing.T) {
	dir := historyRepo(t)
	git(t, dir, "branch", "dev")
	main := git(t, dir, "branch", "--show-current")
	if code, out, errs := run(newApp(), dir, "init"); code != 0 || strings.Contains(out, "dev создана") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if git(t, dir, "branch", "--show-current") != main {
		t.Error("HEAD не переключается, если dev уже есть")
	}
}

func TestInitHelp(t *testing.T) {
	for _, args := range [][]string{{"init", "--help"}, {"init", "-h"}, {"help", "init"}} {
		code, out, errs := run(newApp(), t.TempDir(), args...)
		if code != 0 || errs != "" {
			t.Fatalf("%v: %d %s", args, code, errs)
		}
		if out != strings.TrimRight(initHelp, "\n")+"\n" && out != initHelp {
			t.Errorf("%v: вывод не совпадает со справкой init", args)
		}
	}
}

func TestInitHelpDoesNotInit(t *testing.T) {
	dir := t.TempDir()
	run(newApp(), dir, "init", "--help")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("--help не должен создавать репозиторий")
	}
	if _, err := os.Stat(filepath.Join(dir, ".sky")); err == nil {
		t.Error("--help не должен создавать .sky")
	}
}

func TestHelpUnknownAndPlain(t *testing.T) {
	if code, _, _ := run(newApp(), t.TempDir(), "help", "nope"); code != 2 {
		t.Errorf("help nope: %d", code)
	}
	if code, out, _ := run(newApp(), t.TempDir(), "apply", "--help"); code != 0 || !strings.Contains(out, "sky apply") {
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

func TestCommitCommand(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("правка\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("новый\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.db"), []byte("x"), 0o644)

	code, out, errs := run(newApp(), dir, "commit", "docs: edit readme")
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if !strings.Contains(out, "docs: edit readme") || !strings.Contains(out, "? new.txt") || strings.Contains(out, "ignored.db") {
		t.Errorf("вывод: %s", out)
	}
	if files := git(t, dir, "show", "--name-only", "--format=", "HEAD"); files != "README.md" {
		t.Errorf("в коммите: %q", files)
	}
	if code, _, errs := run(newApp(), dir, "commit", "docs: again"); code != 1 {
		t.Errorf("повторно: %d %s", code, errs)
	}
}

func TestCommitCommandArgs(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("правка\n"), 0o644)
	for _, args := range [][]string{{"commit"}, {"commit", "feat:", "two"}, {"commit", "no type"}, {"commit", "feat: " + strings.Repeat("x", 73)}} {
		if code, _, _ := run(newApp(), dir, args...); code != 1 {
			t.Errorf("%v: ждали отказ", args)
		}
	}
	if st := git(t, dir, "status", "--porcelain", "--untracked-files=no"); st == "" {
		t.Error("отказ не должен коммитить")
	}
}

func TestCommitNeedsInit(t *testing.T) {
	dir := gitDir(t)
	if code, _, errs := run(newApp(), dir, "commit", "fix: x"); code != 1 {
		t.Errorf("%d %s", code, errs)
	}
}

func TestCommitNewFlag(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("новый\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "x.db"), []byte("x"), 0o644) // в .gitignore из шаблона

	if code, _, errs := run(newApp(), dir, "commit", "feat: add new"); code != 1 {
		t.Fatalf("без --new при одних новых файлах: %d %s", code, errs)
	}
	for _, args := range [][]string{{"commit", "--new", "feat: add new"}} {
		code, out, errs := run(newApp(), dir, args...)
		if code != 0 {
			t.Fatalf("%d %s %s", code, out, errs)
		}
	}
	if files := git(t, dir, "show", "--name-only", "--format=", "HEAD"); files != "new.txt" {
		t.Errorf("в коммите: %q", files)
	}
	// флаг можно ставить после сообщения
	os.WriteFile(filepath.Join(dir, "n2.txt"), []byte("2\n"), 0o644)
	if code, _, errs := run(newApp(), dir, "commit", "feat: add n2", "--new"); code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}

func TestRollbackCommand(t *testing.T) {
	dir, inbox := project(t)
	zipPatch(t, filepath.Join(inbox, "patch-1.zip"), map[string]string{
		"project/a.txt": "a\n", ".sky/operation/commit": "feat: add a\n",
	})
	if code, out, errs := run(newApp(), dir, "apply"); code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	code, out, _ := run(newApp(), dir, "rollback", "list")
	if code != 0 {
		t.Fatalf("list: %d %s", code, out)
	}
	code, out, errs := run(newApp(), dir, "rollback")
	if code != 0 {
		t.Fatalf("rollback: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "a.txt")); err == nil {
		t.Error("a.txt должен быть удалён")
	}
	if n := git(t, dir, "rev-list", "--count", "HEAD"); n != "1" {
		t.Errorf("коммитов: %s", n)
	}
	if code, _, _ := run(newApp(), dir, "rollback", "x"); code != 1 {
		t.Error("неизвестный аргумент")
	}
}

func TestRollbackForce(t *testing.T) {
	dir, inbox := project(t)
	zipPatch(t, filepath.Join(inbox, "patch-1.zip"), map[string]string{
		"project/a.txt": "a1\n", ".sky/operation/commit": "feat: add a\n",
	})
	zipPatch(t, filepath.Join(inbox, "patch-2.zip"), map[string]string{
		"project/a.txt": "a2\n", "project/b.txt": "b\n", ".sky/operation/commit": "feat: add b\n",
	})
	if code, out, errs := run(newApp(), dir, "apply"); code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	head := git(t, dir, "rev-parse", "HEAD")
	code, out, errs := run(newApp(), dir, "rollback", "--force", "2")
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "a.txt")); err == nil {
		t.Error("a.txt появился патчем 1, после force 2 его быть не должно")
	}
	if _, err := os.Stat(filepath.Join(dir, "project", "b.txt")); err == nil {
		t.Error("b.txt должен быть удалён")
	}
	if git(t, dir, "rev-parse", "HEAD") != head {
		t.Error("force не трогает коммиты")
	}
	// force откат — запись; обычный откат возвращает файлы
	if code, _, errs := run(newApp(), dir, "rollback"); code != 1 && code != 0 {
		t.Fatal(errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "project", "a.txt")); string(b) != "a2\n" {
		t.Errorf("откат force-записи должен вернуть файлы: %q", b)
	}
	// у записи отката снимка нет
	run(newApp(), dir, "rollback")
	if code, _, errs := run(newApp(), dir, "rollback", "--force"); code != 1 {
		t.Errorf("%d %s", code, errs)
	}
}

func TestManualCommitIsRecord(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	before := git(t, dir, "rev-parse", "HEAD")
	readme := filepath.Join(dir, "README.md")
	orig, _ := os.ReadFile(readme)
	os.WriteFile(readme, []byte("правка\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n\n"), 0o644)
	code, out, errs := run(newApp(), dir, "commit", "--new", "docs: edit")
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if code, o, _ := run(newApp(), dir, "rollback", "list"); code != 0 || o == "" {
		t.Errorf("list: %d %s", code, o)
	}
	// --keep: коммит снят, файлы на месте
	if code, o, errs := run(newApp(), dir, "rollback", "--keep"); code != 0 {
		t.Fatalf("keep: %d %s %s", code, o, errs)
	}
	if git(t, dir, "rev-parse", "HEAD") != before {
		t.Error("коммит должен быть снят")
	}
	if b, _ := os.ReadFile(readme); string(b) != "правка\n" {
		t.Errorf("--keep не должен трогать файлы: %q", b)
	}
	// «amend»: новый коммит и откат без --keep возвращает файлы «до»
	if code, _, errs := run(newApp(), dir, "commit", "--new", "docs: edit again"); code != 0 {
		t.Fatal(errs)
	}
	if code, o, errs := run(newApp(), dir, "rollback"); code != 0 {
		t.Fatalf("%d %s %s", code, o, errs)
	}
	if b, _ := os.ReadFile(readme); string(b) != string(orig) {
		t.Errorf("откат ручного коммита возвращает файлы: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err == nil {
		t.Error("new.txt должен быть удалён откатом")
	}
	if git(t, dir, "rev-parse", "HEAD") != before {
		t.Error("коммит должен быть снят")
	}
}

func TestRollbackKeepArgs(t *testing.T) {
	dir, _ := project(t)
	if code, _, _ := run(newApp(), dir, "rollback", "--keep", "--force"); code != 1 {
		t.Error("--keep и --force несовместимы")
	}
	if code, _, errs := run(newApp(), dir, "rollback", "--keep"); code != 1 {
		t.Errorf("%d %s", code, errs)
	}
}

func TestRollbackArgs(t *testing.T) {
	dir, _ := project(t)
	for _, a := range [][]string{{"1", "2"}, {"list", "--force"}, {"list", "3"}, {"-1"}} {
		if code, _, _ := run(newApp(), dir, append([]string{"rollback"}, a...)...); code != 1 {
			t.Errorf("%v: код %d", a, code)
		}
	}
}

func TestRollbackEmpty(t *testing.T) {
	dir, _ := project(t)
	if code, _, errs := run(newApp(), dir, "rollback"); code != 1 {
		t.Errorf("%d %s", code, errs)
	}
	if code, out, _ := run(newApp(), dir, "rollback", "list"); code != 0 {
		t.Errorf("list: %d %s", code, out)
	}
}

func TestLookCommand(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	l := repo.At(dir)
	if _, err := os.Stat(filepath.Join(l.Foryou, "status.md")); err != nil {
		t.Fatal("init должен создавать foryou")
	}
	fi, _ := os.Stat(filepath.Join(l.Foryou, "status.md"))
	if fi.Mode().Perm()&0o222 != 0 {
		t.Errorf("foryou только для чтения: %v", fi.Mode())
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("правка\n"), 0o644)
	if code, _, errs := run(newApp(), dir, "commit", "docs: edit"); code != 0 {
		t.Fatal(errs)
	}
	h, _ := os.ReadFile(filepath.Join(l.Foryou, "history.md"))
	if !strings.Contains(string(h), "docs: edit") {
		t.Errorf("commit должен обновлять foryou:\n%s", h)
	}
	os.WriteFile(filepath.Join(l.Foryou, "junk.md"), nil, 0o644)
	code, out, _ := run(newApp(), dir, "look")
	if code != 0 || !strings.Contains(out, "status.md") {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(l.Foryou, "junk.md")); err == nil {
		t.Error("посторонний файл в foryou должен быть убран")
	}
	if code, _, _ := run(newApp(), dir, "look", "x"); code != 1 {
		t.Error("аргументы не принимаются")
	}
}

func TestUpCommand(t *testing.T) {
	t.Setenv("SKY_NO_CLIP", "1")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	dir := gitDir(t)
	run(newApp(), dir, "init")
	code, out, errs := run(newApp(), dir, "up")
	if code != 0 || !strings.Contains(out, "-up.zip") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	l := repo.At(dir)
	run(newApp(), dir, "up")
	zs, _ := filepath.Glob(filepath.Join(tmp, "sky-up", "*-up.zip"))
	if len(zs) != 1 {
		t.Fatalf("архивов: %v", zs)
	}
	if bs, _ := filepath.Glob(filepath.Join(l.Backups, "*")); len(bs) != 0 {
		t.Errorf("up не пишет в backups/: %v", bs)
	}
	md, err := os.ReadFile(filepath.Join(l.Foryou, "archive.md"))
	if err != nil || len(md) == 0 {
		t.Fatalf("archive.md: %v %s", err, md)
	}
	// обновление foryou после других команд не должно стирать archive.md
	run(newApp(), dir, "look")
	if _, err := os.Stat(filepath.Join(l.Foryou, "archive.md")); err != nil {
		t.Error("look стёр archive.md")
	}
	if code, _, _ := run(newApp(), dir, "up", "x"); code != 1 {
		t.Error("аргументы не принимаются")
	}
	// архив не попадает ни в inbox-маску, ни в записи rollback
	if code, _, errs := run(newApp(), dir, "rollback"); code != 1 {
		t.Errorf("backups/ не должна считаться записью stages: %d %s", code, errs)
	}
}

func TestApplyMakesBackupBeforePatch(t *testing.T) {
	dir, inbox := project(t)
	os.WriteFile(filepath.Join(dir, "project", "keep.txt"), []byte("прежнее\n"), 0o644)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "chore: keep")
	zipPatch(t, filepath.Join(inbox, "patch-1.zip"), map[string]string{"project/keep.txt": "новое\n"})
	code, out, errs := run(newApp(), dir, "apply")
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	zs, _ := filepath.Glob(filepath.Join(repo.At(dir).Backups, "*-before-patch-1.zip"))
	if len(zs) != 1 {
		t.Fatalf("снимков: %v", zs)
	}
}

func TestInitRefusesWithoutGitIdentity(t *testing.T) {
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.useConfigOnly")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	dir := t.TempDir()
	code, _, errs := run(newApp(), dir, "init")
	if code != 1 {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("до отказа ничего не создаётся")
	}
	if _, err := os.Stat(filepath.Join(dir, ".sky")); err == nil {
		t.Error(".sky/ не должна появиться")
	}
}

func TestCommitUnknownFlag(t *testing.T) {
	dir := gitDir(t)
	initWithFiles(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("правка\n"), 0o644)
	if code, _, errs := run(newApp(), dir, "commit", "--all", "docs: x"); code != 1 {
		t.Errorf("%d %s", code, errs)
	}
}
