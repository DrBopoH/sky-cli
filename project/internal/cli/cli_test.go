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
	if g, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); !strings.Contains(string(g), ".sky/") {
		t.Fatalf(".gitignore: %q", g)
	}
	if !strings.Contains(out, "project/") {
		t.Fatalf("нет предупреждения про отсутствующую папку: %s", out)
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

func TestInitErrors(t *testing.T) {
	if code, _, errs := run(New(), t.TempDir(), "init"); code != 1 || !strings.Contains(errs, "git") {
		t.Fatalf("вне git: %d %s", code, errs)
	}
	if code, _, _ := run(New(), gitDir(t), "init", "../x"); code != 1 {
		t.Fatal("scope вне репозитория должен падать")
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
