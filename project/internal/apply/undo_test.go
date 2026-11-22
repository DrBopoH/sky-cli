package apply

import (
	"strings"
	"testing"
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
	res, target, err := Undo(e.root, e.stages)
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
	if _, _, err := Undo(e.root, e.stages); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Undo(e.root, e.stages); err != nil {
		t.Fatal(err)
	}
	if b, _ := e.read("a.txt"); b != "новый\n" {
		t.Errorf("повторный откат должен вернуть патч: %q", b)
	}
	if _, ok := e.read("n.txt"); !ok {
		t.Error("n.txt должен вернуться")
	}
	list, _ := Entries(e.stages)
	if len(list) != 3 {
		t.Errorf("записей: %d", len(list))
	}
}

func TestUndoKeepsManualEditsInRedo(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.txt", "старый\n")
	e.run(e.patch(t, "patch-a.zip", entry{name: "a.txt", body: "новый\n"}))
	e.write(t, "a.txt", "правка руками\n")
	Undo(e.root, e.stages)
	Undo(e.root, e.stages)
	if b, _ := e.read("a.txt"); b != "правка руками\n" {
		t.Errorf("ручная правка потеряна: %q", b)
	}
}

func TestUndoNoEntries(t *testing.T) {
	e := newEnv(t)
	if _, _, err := Undo(e.root, e.stages); err == nil || !strings.Contains(err.Error(), "нет записей") {
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
	res, _, err := Undo(e.root, e.stages)
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
	if _, _, err := Undo(e.root, e.stages); err != nil {
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
	if _, _, err := Undo(e.root, e.stages); err == nil || !strings.Contains(err.Error(), "не вершина") {
		t.Fatalf("ждали отказ: %v", err)
	}
	if _, ok := e.read("x.txt"); !ok || g(t, e.root, "rev-parse", "HEAD") != head {
		t.Error("при отказе ничего не меняется")
	}
	if list, _ := Entries(e.stages); len(list) != 1 {
		t.Errorf("при отказе новой записи быть не должно: %d", len(list))
	}
}

func TestEntries(t *testing.T) {
	e := gitEnv(t)
	e.run(e.patch(t, "patch-a.zip", entry{name: "x.txt", body: "x\n"}, entry{name: ".sky/operation/commit", body: "feat: x\n"}))
	e.run(e.patch(t, "patch-b.zip", entry{name: "y.txt", body: "y\n"}))
	list, err := Entries(e.stages)
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
