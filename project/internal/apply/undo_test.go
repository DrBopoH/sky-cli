package apply_test

import (
	"strings"
	"testing"

	"github.com/DrBopoH/sky-cli/internal/apply"
)

func TestUndoRestoresFiles(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.txt", "старый a\n")
	e.write(t, "gone.txt", "будет удалён\n")
	p := e.patch(t, "patch-a.zip",
		entry{name: "a.txt", body: "новый a\n"},
		entry{name: "dir/new.txt", body: "новый\n"},
		entry{name: ".sky/operation/delete", body: "gone.txt\n"},
	)
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	res, es, err := apply.Undo(e.root, e.stages, 1)
	var target apply.Entry
	if len(es) > 0 {
		target = es[0]
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target.Name, "patch-a") {
		t.Errorf("откатывали не ту запись: %s", target.Name)
	}
	if b, _ := e.read("a.txt"); b != "старый a\n" {
		t.Errorf("a.txt: %q", b)
	}
	if b, ok := e.read("gone.txt"); !ok || b != "будет удалён\n" {
		t.Errorf("gone.txt: %q %v", b, ok)
	}
	if _, ok := e.read("dir/new.txt"); ok {
		t.Error("созданный патчем файл должен быть удалён")
	}
	marks := map[string]string{}
	for _, l := range res.Lines {
		marks[l.Path] = l.Mark
	}
	if marks["a.txt"] != "~" || marks["gone.txt"] != "+" || marks["dir/new.txt"] != "-" {
		t.Errorf("отметки: %v", marks)
	}
}

func TestUndoTwiceRedoes(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.txt", "старый\n")
	if _, err := e.run(e.patch(t, "patch-a.zip", entry{name: "a.txt", body: "новый\n"}, entry{name: "n.txt", body: "n\n"})); err != nil {
		t.Fatal(err)
	}
	if _, _, err := apply.Undo(e.root, e.stages, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := apply.Undo(e.root, e.stages, 1); err != nil {
		t.Fatal(err)
	}
	if b, _ := e.read("a.txt"); b != "новый\n" {
		t.Errorf("повторный откат должен вернуть патч: %q", b)
	}
	if _, ok := e.read("n.txt"); !ok {
		t.Error("n.txt должен вернуться")
	}
	list, _ := apply.Entries(e.stages)
	if len(list) != 3 {
		t.Errorf("записей: %d", len(list))
	}
}

func TestUndoKeepsManualEditsInRedo(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.txt", "старый\n")
	e.run(e.patch(t, "patch-a.zip", entry{name: "a.txt", body: "новый\n"}))
	e.write(t, "a.txt", "правка руками\n")
	apply.Undo(e.root, e.stages, 1)
	apply.Undo(e.root, e.stages, 1)
	if b, _ := e.read("a.txt"); b != "правка руками\n" {
		t.Errorf("ручная правка потеряна: %q", b)
	}
}

func TestUndoNoEntries(t *testing.T) {
	e := newEnv(t)
	if _, _, err := apply.Undo(e.root, e.stages, 1); err == nil || !strings.Contains(err.Error(), "нет записей") {
		t.Errorf("%v", err)
	}
}

func TestUndoRemovesCommit(t *testing.T) {
	e := gitEnv(t)
	before := g(t, e.root, "rev-parse", "HEAD")
	p := e.patch(t, "patch-a.zip",
		entry{name: "base.txt", body: "изменён\n"},
		entry{name: "new.txt", body: "новый\n"},
		entry{name: ".sky/operation/commit", body: "feat: change\n"},
	)
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	res, _, err := apply.Undo(e.root, e.stages, 1)
	if err != nil {
		t.Fatal(err)
	}
	if g(t, e.root, "rev-parse", "HEAD") != before {
		t.Error("коммит должен быть снят")
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "" {
		t.Errorf("дерево после отката не чистое: %q", st)
	}
	if !strings.Contains(strings.Join(res.Notes, "|"), "снят") {
		t.Errorf("пометки: %v", res.Notes)
	}
	// откат отката: файлы возвращаются, коммит нет
	if _, _, err := apply.Undo(e.root, e.stages, 1); err != nil {
		t.Fatal(err)
	}
	if b, _ := e.read("base.txt"); b != "изменён\n" {
		t.Errorf("base.txt: %q", b)
	}
	if g(t, e.root, "rev-parse", "HEAD") != before {
		t.Error("повторный откат не должен восстанавливать коммит")
	}
}

func TestUndoRefusesWhenCommitIsNotHead(t *testing.T) {
	e := gitEnv(t)
	p := e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"}, entry{name: ".sky/operation/commit", body: "feat: x\n"})
	if _, err := e.run(p); err != nil {
		t.Fatal(err)
	}
	e.write(t, "y.txt", "y\n")
	g(t, e.root, "add", "y.txt")
	g(t, e.root, "commit", "-q", "-m", "chore: later")
	head := g(t, e.root, "rev-parse", "HEAD")
	if _, _, err := apply.Undo(e.root, e.stages, 1); err == nil || !strings.Contains(err.Error(), "вершине ветки") {
		t.Fatalf("ждали отказ: %v", err)
	}
	if _, ok := e.read("x.txt"); !ok || g(t, e.root, "rev-parse", "HEAD") != head {
		t.Error("при отказе ничего не меняется")
	}
	if list, _ := apply.Entries(e.stages); len(list) != 1 {
		t.Errorf("при отказе новой записи быть не должно: %d", len(list))
	}
}

func TestEntries(t *testing.T) {
	e := gitEnv(t)
	e.run(e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"}, entry{name: ".sky/operation/commit", body: "feat: x\n"}))
	e.run(e.patch(t, "patch-b.zip", entry{name: "y.txt", body: "y\n"}))
	list, err := apply.Entries(e.stages)
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].Source != "patch-a.zip" || list[0].Files != 1 || len(list[0].Commit) != 40 {
		t.Errorf("первая: %+v", list[0])
	}
	if list[1].Commit != "" || list[1].Num != 2 {
		t.Errorf("вторая: %+v", list[1])
	}
}

func TestUndoNPatches(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.txt", "0\n")
	for i, body := range []string{"1\n", "2\n", "3\n"} {
		name := string(rune('a'+i)) + ".zip"
		if _, err := e.run(e.patch(t, "patch-"+name, entry{name: "a.txt", body: body}, entry{name: "f" + strings.TrimSpace(body) + ".txt", body: body})); err != nil {
			t.Fatal(err)
		}
	}
	res, es, err := apply.Undo(e.root, e.stages, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("записей: %d", len(es))
	}
	if b, _ := e.read("a.txt"); b != "1\n" {
		t.Errorf("a.txt после отката 2: %q", b)
	}
	if _, ok := e.read("f2.txt"); ok {
		t.Error("f2 должен исчезнуть")
	}
	if _, ok := e.read("f1.txt"); !ok {
		t.Error("f1 должен остаться")
	}
	if !strings.Contains(res.Stage, "rollback-0002_0003") {
		t.Errorf("имя записи: %s", res.Stage)
	}
	if _, _, err := apply.Undo(e.root, e.stages, 17); err == nil {
		t.Error("глубина больше 16 должна отклоняться")
	}
	if _, _, err := apply.Undo(e.root, e.stages, 99); err == nil {
		t.Error("глубина больше числа записей должна отклоняться")
	}
}

func TestUndoNCommits(t *testing.T) {
	e := gitEnv(t)
	base := g(t, e.root, "rev-parse", "HEAD")
	for _, n := range []string{"x", "y", "z"} {
		if _, err := e.run(e.patch(t, "patch-"+n+".zip", entry{name: n + ".txt", body: n + "\n"}, entry{name: ".sky/operation/commit", body: "feat: " + n + "\n"})); err != nil {
			t.Fatal(err)
		}
	}
	res, _, err := apply.Undo(e.root, e.stages, 3)
	if err != nil {
		t.Fatal(err)
	}
	if g(t, e.root, "rev-parse", "HEAD") != base {
		t.Error("три коммита должны быть сняты")
	}
	if !strings.Contains(strings.Join(res.Notes, "|"), "3") {
		t.Errorf("пометки: %v", res.Notes)
	}
	if st := g(t, e.root, "status", "--porcelain"); st != "" {
		t.Errorf("дерево не чистое: %q", st)
	}
}

func TestHistoryPrunedTo16(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < apply.KeepHistory+3; i++ {
		if _, err := e.run(e.patch(t, "patch-n.zip", entry{name: "a.txt", body: string(rune('a'+i%20)) + "\n"})); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := apply.Entries(e.stages)
	if len(list) != apply.KeepHistory {
		t.Fatalf("записей: %d", len(list))
	}
	if list[len(list)-1].Num != apply.KeepHistory+3 {
		t.Errorf("новейшая запись потеряна: %d", list[len(list)-1].Num)
	}
}

func TestBackupNoted(t *testing.T) {
	e := newEnv(t)
	res, err := e.run(e.patch(t, "patch-a.zip", entry{name: "a.txt", body: "x\n"}))
	if err != nil {
		t.Fatal(err)
	}
	apply.NoteBackup(res.Stage, "0001-x-before-patch-a.zip")
	list, _ := apply.Entries(e.stages)
	if list[0].Backup != "0001-x-before-patch-a.zip" {
		t.Errorf("backup: %q", list[0].Backup)
	}
}
