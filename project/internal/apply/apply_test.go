package apply_test

import (
	"archive/zip"
	"bytes"
	"github.com/DrBopoH/sky-cli/internal/guard"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/ops"
)

type entry struct {
	name string
	body string
	mode os.FileMode
}

func mkZip(t *testing.T, path string, entries ...entry) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode == 0 {
			e.mode = 0o644
		}
		h.SetMode(e.mode)
		w, _ := zw.CreateHeader(h)
		w.Write([]byte(e.body))
	}
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type env struct {
	root, stages, inbox string
}

func newEnv(t *testing.T) env {
	t.Helper()
	root := t.TempDir()
	e := env{root: root, stages: filepath.Join(root, ".sky", "stages"), inbox: t.TempDir()}
	os.MkdirAll(e.stages, 0o755)
	os.MkdirAll(filepath.Join(root, ".sky", "clouds"), 0o755)
	return e
}

func (e env) write(t *testing.T, rel, body string) {
	t.Helper()
	p := filepath.Join(e.root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e env) read(rel string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(e.root, rel))
	return string(b), err == nil
}

func (e env) patch(t *testing.T, name string, entries ...entry) string {
	p := filepath.Join(e.inbox, name)
	mkZip(t, p, entries...)
	return p
}

func (e env) run(p string) (apply.Result, error) {
	plan, err := ops.Build(e.root, p)
	if err != nil {
		return apply.Result{}, err
	}
	return apply.Run(e.root, e.stages, p, plan)
}

// snapshotTree возвращает путь→содержимое всех файлов репозитория, кроме .sky/stages.
func (e env) snapshotTree(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(e.root, func(p string, fi os.FileInfo, err error) error {
		rel, _ := filepath.Rel(e.root, p)
		if fi.IsDir() {
			if strings.HasPrefix(filepath.ToSlash(rel), ".sky/stages") {
				return filepath.SkipDir
			}
			return nil
		}
		b, _ := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return out
}

func TestApplyCreateModifyDeleteSame(t *testing.T) {
	e := newEnv(t)
	e.write(t, "keep.txt", "keep")
	e.write(t, "mod.txt", "old")
	e.write(t, "gone/inner.txt", "bye")
	p := e.patch(t, "patch-a.zip",
		entry{name: "new/dir/file.txt", body: "n"},
		entry{name: "mod.txt", body: "new"},
		entry{name: "keep.txt", body: "keep"},
		entry{name: ".sky/operation/delete", body: "# удалить\ngone/inner.txt\nabsent.txt\n"},
	)
	res, err := e.run(p)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := e.read("new/dir/file.txt"); s != "n" {
		t.Error("новый файл")
	}
	if s, _ := e.read("mod.txt"); s != "new" {
		t.Error("изменённый файл")
	}
	if _, ok := e.read("gone/inner.txt"); ok {
		t.Error("файл не удалён")
	}
	if _, err := os.Stat(filepath.Join(e.root, "gone")); err == nil {
		t.Error("пустая папка должна исчезнуть")
	}
	if _, err := os.Stat(filepath.Join(e.root, ".sky", "operation")); err == nil {
		t.Error("operation/ не должна попадать на диск")
	}
	marks := map[string]string{}
	for _, l := range res.Lines {
		marks[l.Path] = l.Mark
	}
	want := map[string]string{"new/dir/file.txt": "+", "mod.txt": "~", "gone/inner.txt": "-", "keep.txt": "="}
	for k, v := range want {
		if marks[k] != v {
			t.Errorf("%s: метка %q, нужна %q", k, marks[k], v)
		}
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "absent.txt") {
		t.Errorf("заметки: %v", res.Notes)
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("патч должен уйти из inbox")
	}
	if _, err := os.Stat(filepath.Join(res.Stage, "patch-a.zip")); err != nil {
		t.Error("патч должен лежать в записи stages")
	}
	man, _ := os.ReadFile(filepath.Join(res.Stage, "manifest.txt"))
	for _, want := range []string{"created\tnew/dir/file.txt", "modified\tmod.txt", "deleted\tgone/inner.txt"} {
		if !strings.Contains(string(man), want) {
			t.Errorf("в manifest нет %q:\n%s", want, man)
		}
	}
}

func TestRestoreReturnsBefore(t *testing.T) {
	e := newEnv(t)
	e.write(t, "mod.txt", "old")
	e.write(t, "del/x.txt", "x")
	before := e.snapshotTree(t)
	res, err := e.run(e.patch(t, "patch.zip",
		entry{name: "mod.txt", body: "new"},
		entry{name: "added.txt", body: "a"},
		entry{name: ".sky/operation/delete", body: "del/x.txt\n"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Restore(e.root, res.Stage); err != nil {
		t.Fatal(err)
	}
	after := e.snapshotTree(t)
	if len(after) != len(before) {
		t.Fatalf("после отката другой набор файлов: %v vs %v", after, before)
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s: %q, было %q", k, after[k], v)
		}
	}
}

func TestExecBitPreserved(t *testing.T) {
	e := newEnv(t)
	if _, err := e.run(e.patch(t, "patch.zip", entry{name: "run.sh", body: "#!/bin/sh\n", mode: 0o755}, entry{name: "doc.txt", body: "d"})); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(e.root, "run.sh")); fi.Mode().Perm()&0o111 == 0 {
		t.Error("бит исполняемости потерян")
	}
	if fi, _ := os.Stat(filepath.Join(e.root, "doc.txt")); fi.Mode().Perm()&0o111 != 0 {
		t.Error("обычный файл не должен стать исполняемым")
	}
}

func TestCloudsAllowed(t *testing.T) {
	e := newEnv(t)
	if _, err := e.run(e.patch(t, "patch.zip", entry{name: ".sky/clouds/plan.md", body: "p"})); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.read(".sky/clouds/plan.md"); s != "p" {
		t.Error("заметка не записана")
	}
	e.run(e.patch(t, "patch2.zip", entry{name: ".sky/operation/delete", body: ".sky/clouds/plan.md\n"}))
	if _, err := os.Stat(filepath.Join(e.root, ".sky", "clouds")); err != nil {
		t.Error("папка clouds/ не должна исчезать при удалении заметок")
	}
}

func TestRejectedPatchesChangeNothing(t *testing.T) {
	bad := map[string][]entry{
		"абсолютный путь":   {{name: "/etc/x", body: "x"}},
		"выход вверх":       {{name: "../x", body: "x"}},
		"ненормализованный": {{name: "a/../b", body: "x"}},
		".git":              {{name: ".git/config", body: "x"}},
		"вложенный .git":    {{name: "sub/.git/x", body: "x"}},
		"readme":            {{name: ".sky/readme/readme.md", body: "x"}},
		"foryou":            {{name: ".sky/foryou/status.md", body: "x"}},
		"stages":            {{name: ".sky/stages/0001/x", body: "x"}},
		"sky.conf":          {{name: ".sky/sky.conf", body: "x"}},
		"база":              {{name: "data/app.db", body: "x"}},
		".env":              {{name: ".env", body: "x"}},
		"ключ":              {{name: "k/id.pem", body: "x"}},
		"обратный слэш":     {{name: `a\b.txt`, body: "x"}},
		"симлинк":           {{name: "link", body: "target", mode: os.ModeSymlink | 0o777}},
		"неизвестная операция": {{name: "ok.txt", body: "ok"}, {name: ".sky/operation/rm", body: "x\n"}},
		"операция в подпапке":  {{name: ".sky/operation/sub/delete", body: "x\n"}},
		"плохая запись":        {{name: "ok.txt", body: "ok"}, {name: ".sky/operation/delete", body: "a\tb\n"}},
		"delete защищённого":   {{name: "ok.txt", body: "ok"}, {name: ".sky/operation/delete", body: ".sky/sky.conf\n"}},
		"delete папки":         {{name: "ok.txt", body: "ok"}, {name: ".sky/operation/delete", body: "dir\n"}},
		"путь дважды":          {{name: "a.txt", body: "1"}, {name: ".sky/operation/delete", body: "a.txt\n"}},
		"файл и папка":         {{name: "a", body: "1"}, {name: "a/b.txt", body: "2"}},
	}
	for name, entries := range bad {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.write(t, "a.txt", "orig")
			e.write(t, "dir/f.txt", "f")
			before := e.snapshotTree(t)
			p := e.patch(t, "patch.zip", entries...)
			if _, err := e.run(p); err == nil {
				t.Fatal("патч должен быть отклонён")
			}
			after := e.snapshotTree(t)
			if len(after) != len(before) {
				t.Fatalf("дерево изменилось: %v", after)
			}
			for k, v := range before {
				if after[k] != v {
					t.Fatalf("%s изменён", k)
				}
			}
			if _, err := os.Stat(p); err != nil {
				t.Error("отклонённый патч должен остаться в inbox")
			}
			if es, _ := os.ReadDir(e.stages); len(es) != 0 {
				t.Errorf("запись в stages не должна оставаться: %v", es)
			}
		})
	}
}

func TestWriteFailureRollsBack(t *testing.T) {
	e := newEnv(t)
	e.write(t, "first.txt", "old")
	e.write(t, "a", "я файл, а патч ждёт папку")
	before := e.snapshotTree(t)
	p := e.patch(t, "patch.zip",
		entry{name: "first.txt", body: "new"},
		entry{name: "fresh.txt", body: "f"},
		entry{name: "a/b.txt", body: "не получится"},
	)
	if _, err := e.run(p); err == nil {
		t.Fatal("ждали ошибку записи")
	}
	after := e.snapshotTree(t)
	if len(after) != len(before) {
		t.Fatalf("откат не вернул набор файлов: %v", after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s: %q, было %q", k, after[k], v)
		}
	}
	if _, err := os.Stat(p); err != nil {
		t.Error("патч должен остаться в inbox")
	}
	if es, _ := os.ReadDir(e.stages); len(es) != 0 {
		t.Errorf("после отката запись stages должна исчезнуть: %v", es)
	}
}

func TestSymlinkEscape(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.root, "link")); err != nil {
		t.Skip("symlink недоступен")
	}
	if _, err := e.run(e.patch(t, "patch.zip", entry{name: "link/x.txt", body: "x"})); err == nil {
		t.Fatal("запись через symlink за пределы репозитория должна отклоняться")
	}
	if _, err := os.Stat(filepath.Join(outside, "x.txt")); err == nil {
		t.Fatal("файл записан за пределами репозитория")
	}
	if err := os.Symlink("target", filepath.Join(e.root, "lnk.txt")); err == nil {
		if _, err := e.run(e.patch(t, "patch2.zip", entry{name: "lnk.txt", body: "x"})); err == nil {
			t.Fatal("запись поверх symlink должна отклоняться")
		}
	}
}

func TestStageNumbering(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"patch-one.zip", "patch-two.zip"} {
		if _, err := e.run(e.patch(t, n, entry{name: n + ".txt", body: "x"})); err != nil {
			t.Fatal(err)
		}
	}
	es, _ := os.ReadDir(e.stages)
	if len(es) != 2 || !strings.HasPrefix(es[0].Name(), "0001-") || !strings.HasPrefix(es[1].Name(), "0002-") {
		t.Fatalf("%v", es)
	}
	if !strings.HasSuffix(es[1].Name(), "patch-two") {
		t.Errorf("имя патча в записи: %s", es[1].Name())
	}
}

func TestGuardPath(t *testing.T) {
	for _, ok := range []string{"a.txt", "dir/sub/f.go", ".github/ci.yml", ".sky/clouds/n.md", ".sky/clouds/d/n.md", "имя с пробелом.txt", ".gitignore", ".env.example"} {
		if err := guard.Path(ok); err != nil {
			t.Errorf("%q должен проходить: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/a", "../a", "a/../b", "./a", "a//b", ".git/x", ".sky", ".sky/clouds", ".sky/sky.conf", "x.sqlite-wal", "id.key", ".env"} {
		if guard.Path(bad) == nil {
			t.Errorf("%q должен отклоняться", bad)
		}
	}
}
