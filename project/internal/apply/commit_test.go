package apply

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitEnv(t *testing.T) env {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
	} {
		t.Setenv(k, v)
	}
	e := newEnv(t)
	if out, err := exec.Command("git", "-C", e.root, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Skipf("git недоступен: %v %s", err, out)
	}
	e.write(t, ".gitignore", ".sky/\n.claude\n")
	e.write(t, "base.txt", "base\n")
	g(t, e.root, "add", ".gitignore", "base.txt")
	g(t, e.root, "commit", "-q", "-m", "chore: base")
	return e
}

func g(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitOnlyPatchPaths(t *testing.T) {
	e := gitEnv(t)
	e.write(t, "other.txt", "чужое, не в патче\n")
	g(t, e.root, "add", "other.txt") // в индексе до патча — в коммит попасть не должно
	p := e.patch(t, "patch-a.zip",
		entry{name: "a/new.txt", body: "новый\n"},
		entry{name: "base.txt", body: "base 2\n"},
		entry{name: ".sky/operation/commit", body: "feat: add new file\n"},
	)
	res, err := e.run(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.Commit, " feat: add new file") {
		t.Fatalf("Commit: %q", res.Commit)
	}
	if got := g(t, e.root, "log", "-1", "--format=%s"); got != "feat: add new file" {
		t.Errorf("сообщение: %q", got)
	}
	files := g(t, e.root, "show", "--name-only", "--format=", "HEAD")
	if files != "a/new.txt\nbase.txt" {
		t.Errorf("в коммите: %q", files)
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "A  other.txt" {
		t.Errorf("чужое в индексе должно остаться как было: %q", st)
	}
}

func TestCommitWithDelete(t *testing.T) {
	e := gitEnv(t)
	p := e.patch(t, "patch-a.zip",
		entry{name: ".sky/operation/delete", body: "base.txt\n"},
		entry{name: "x.txt", body: "x\n"},
		entry{name: ".sky/operation/commit", body: "refactor: drop base\n"},
	)
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	if files := g(t, e.root, "show", "--name-status", "--format=", "HEAD"); files != "D\tbase.txt\nA\tx.txt" {
		t.Errorf("в коммите: %q", files)
	}
	if _, ok := e.read("base.txt"); ok {
		t.Error("base.txt должен быть удалён")
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}

func TestCommitSkipsIgnoredPaths(t *testing.T) {
	e := gitEnv(t)
	p := e.patch(t, "patch-a.zip",
		entry{name: ".claude", body: "заметки\n"},
		entry{name: ".sky/clouds/n.md", body: "n\n"},
		entry{name: "f.txt", body: "f\n"},
		entry{name: ".sky/operation/commit", body: "docs: add f\n"},
	)
	res, err := e.run(p)
	if err != nil {
		t.Fatal(err)
	}
	if files := g(t, e.root, "show", "--name-only", "--format=", "HEAD"); files != "f.txt" {
		t.Errorf("в коммите: %q", files)
	}
	if len(res.Notes) != 2 || !strings.Contains(strings.Join(res.Notes, "|"), ".claude: в .gitignore") {
		t.Errorf("пометки: %v", res.Notes)
	}
	if _, ok := e.read(".claude"); !ok {
		t.Error("игнорируемый файл всё равно должен быть записан")
	}
}

func TestCommitOnlyIgnoredMakesNoCommit(t *testing.T) {
	e := gitEnv(t)
	before := g(t, e.root, "rev-parse", "HEAD")
	p := e.patch(t, "patch-a.zip",
		entry{name: ".claude", body: "заметки\n"},
		entry{name: ".sky/operation/commit", body: "docs: notes\n"},
	)
	res, err := e.run(p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Commit != "" || g(t, e.root, "rev-parse", "HEAD") != before {
		t.Errorf("коммита быть не должно: %q", res.Commit)
	}
}

func TestCommitNothingChanged(t *testing.T) {
	e := gitEnv(t)
	before := g(t, e.root, "rev-parse", "HEAD")
	p := e.patch(t, "patch-a.zip",
		entry{name: "base.txt", body: "base\n"},
		entry{name: ".sky/operation/commit", body: "fix: noop\n"},
	)
	res, err := e.run(p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Commit != "" || g(t, e.root, "rev-parse", "HEAD") != before {
		t.Error("без изменений коммита быть не должно")
	}
	if !strings.Contains(strings.Join(res.Notes, "|"), "не изменились") {
		t.Errorf("пометки: %v", res.Notes)
	}
}

func TestNoCommitRecordNoCommit(t *testing.T) {
	e := gitEnv(t)
	before := g(t, e.root, "rev-parse", "HEAD")
	if _, err := e.run(e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"})); err != nil {
		t.Fatal(err)
	}
	if g(t, e.root, "rev-parse", "HEAD") != before {
		t.Error("без записи commit коммита быть не должно")
	}
}

func TestCommitRejectedMessagesWriteNothing(t *testing.T) {
	e := gitEnv(t)
	long := strings.Repeat("a", 73)
	for name, body := range map[string]string{
		"без типа":       "add stuff\n",
		"неизвестный":    "wip: x\n",
		"нет текста":     "feat: \n",
		"слишком длинно": "feat: " + long + "\n",
		"две записи":     "feat: one\nfix: two\n",
	} {
		p := e.patch(t, "patch-"+strings.ReplaceAll(name, " ", "")+".zip",
			entry{name: "must-not-exist.txt", body: "x"},
			entry{name: ".sky/operation/commit", body: body},
		)
		if _, err := e.run(p); err == nil {
			t.Errorf("%s: ждали отказ", name)
		}
		if _, ok := e.read("must-not-exist.txt"); ok {
			t.Errorf("%s: файл записан при отказе", name)
		}
		os.Remove(p)
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "" {
		t.Errorf("дерево изменилось: %q", st)
	}
}

func TestCommitAcceptsBoundaryLength(t *testing.T) {
	e := gitEnv(t)
	msg := "feat: " + strings.Repeat("я", 72) // 72 символа после типа, считаем руны
	if _, err := e.run(e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x"}, entry{name: ".sky/operation/commit", body: msg + "\n"})); err != nil {
		t.Fatal(err)
	}
}

func TestCommitFailureRollsBack(t *testing.T) {
	e := gitEnv(t)
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "none"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "none"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EMAIL", "")
	exec.Command("git", "-C", e.root, "config", "--local", "user.useConfigOnly", "true").Run()
	before := g(t, e.root, "rev-parse", "HEAD")
	p := e.patch(t, "patch-a.zip",
		entry{name: "base.txt", body: "изменено\n"},
		entry{name: "new.txt", body: "новый\n"},
		entry{name: ".sky/operation/commit", body: "feat: will fail\n"},
	)
	if _, err := e.run(p); err == nil {
		t.Skip("git смог создать коммит без личности в этой среде")
	}
	if b, _ := e.read("base.txt"); b != "base\n" {
		t.Errorf("base.txt не откатился: %q", b)
	}
	if _, ok := e.read("new.txt"); ok {
		t.Error("new.txt не откатился")
	}
	if g(t, e.root, "rev-parse", "HEAD") != before {
		t.Error("HEAD сдвинулся")
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "" {
		t.Errorf("индекс не очищен: %q", st)
	}
	if _, err := os.Stat(p); err != nil {
		t.Error("патч должен остаться на месте после ошибки")
	}
}

func TestCommitOpNewFlagTakesUntracked(t *testing.T) {
	e := gitEnv(t)
	e.write(t, "stray.txt", "чужой новый файл\n")
	e.write(t, "ign.log", "x\n")
	g(t, e.root, "config", "core.excludesfile", "/dev/null")
	p := e.patch(t, "patch-a.zip",
		entry{name: "x.txt", body: "x\n"},
		entry{name: ".sky/operation/commit", body: "feat: x\tnew\n"},
	)
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	files := g(t, e.root, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "x.txt") || !strings.Contains(files, "stray.txt") {
		t.Errorf("в коммите: %s", files)
	}
	// откат удаляет подобранный файл
	if _, _, err := Undo(e.root, e.stages, 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.read("stray.txt"); ok {
		t.Error("stray.txt должен быть удалён откатом")
	}
}

func TestCommitOpWithoutNewLeavesStray(t *testing.T) {
	e := gitEnv(t)
	e.write(t, "stray.txt", "чужой\n")
	p := e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"}, entry{name: ".sky/operation/commit", body: "feat: x\n"})
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	if files := g(t, e.root, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "stray.txt") {
		t.Errorf("без new чужое не берём: %s", files)
	}
}

func TestCommitOpUnknownFlag(t *testing.T) {
	e := gitEnv(t)
	p := e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"}, entry{name: ".sky/operation/commit", body: "feat: x\tall\n"})
	if _, err := e.run(p); err == nil || !strings.Contains(err.Error(), "неизвестный флаг") {
		t.Errorf("%v", err)
	}
}
