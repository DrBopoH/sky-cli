package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type repo struct {
	t   *testing.T
	dir string
}

func (r repo) git(args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.dir}, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r repo) commit(name, msg string) {
	r.t.Helper()
	os.WriteFile(filepath.Join(r.dir, name), []byte(msg+"\n"), 0o644)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func (r repo) exists(branch string) bool {
	return exec.Command("git", "-C", r.dir, "rev-parse", "--verify", "-q", "refs/heads/"+branch).Run() == nil
}

func clone(t *testing.T, bare, name string) repo {
	t.Helper()
	dir := filepath.Join(filepath.Dir(bare), name)
	if out, err := exec.Command("git", "clone", "-q", bare, dir).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	r := repo{t: t, dir: dir}
	r.git("config", "user.name", name)
	r.git("config", "user.email", name+"@t")
	r.git("config", "commit.gpgsign", "false")
	return r
}

// world — origin с dev (1 коммит); me и other — два клона. other дальше пушит в общую область.
func world(t *testing.T) (me, other repo) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "dev", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	other = clone(t, bare, "other")
	other.git("checkout", "-q", "-b", "dev")
	other.commit("a.txt", "chore: init")
	other.git("push", "-q", "origin", "dev")
	me = clone(t, bare, "me")
	me.git("checkout", "-q", "dev")
	return me, other
}

func TestSyncCreatesAndForwards(t *testing.T) {
	me, other := world(t)
	other.commit("b.txt", "feat: b")
	other.git("branch", "build/1125-1020")
	other.git("branch", "api/1125-1020")
	other.git("branch", "wip/1125-1005/x")
	other.git("branch", "topic")
	other.git("push", "-q", "origin", "dev", "build/1125-1020", "api/1125-1020", "wip/1125-1005/x", "topic")
	res, err := Run(Options{Root: me.dir})
	if err != nil {
		t.Fatal(err)
	}
	if me.git("rev-parse", "dev") != other.git("rev-parse", "dev") {
		t.Error("dev должна догнать удалённую (на ней HEAD, ff)")
	}
	for _, b := range []string{"build/1125-1020", "api/1125-1020"} {
		if !me.exists(b) {
			t.Errorf("нет локальной %s", b)
		}
	}
	if me.exists("wip/1125-1005/x") || me.exists("topic") {
		t.Error("wip и посторонние ветки не заводятся")
	}
	states := map[string]State{}
	for _, it := range res.Items {
		states[it.Name] = it.State
	}
	if states["dev"] != Forward || states["build/1125-1020"] != New || states["api/1125-1020"] != New || len(states) != 3 {
		t.Errorf("%v", states)
	}
	// повторно — всё совпадает
	res, _ = Run(Options{Root: me.dir})
	for _, it := range res.Items {
		if it.State != Same {
			t.Errorf("%s: %s", it.Name, it.State)
		}
	}
}

func TestSyncForwardsNotCheckedOutBranch(t *testing.T) {
	me, other := world(t)
	other.git("branch", "build/1125-1020")
	other.git("push", "-q", "origin", "build/1125-1020")
	if _, err := Run(Options{Root: me.dir}); err != nil {
		t.Fatal(err)
	}
	other.git("checkout", "-q", "build/1125-1020")
	other.commit("fix.txt", "fix: x")
	other.git("push", "-q", "origin", "build/1125-1020")
	if _, err := Run(Options{Root: me.dir}); err != nil {
		t.Fatal(err)
	}
	if me.git("rev-parse", "build/1125-1020") != other.git("rev-parse", "HEAD") || me.git("symbolic-ref", "--short", "HEAD") != "dev" {
		t.Error("сборка продвинута без переключения HEAD")
	}
}

func TestSyncLeavesAheadAndDiverged(t *testing.T) {
	me, other := world(t)
	me.commit("mine.txt", "feat: mine") // у me dev впереди
	res, err := Run(Options{Root: me.dir})
	if err != nil || res.Items[0].State != Ahead || res.Items[0].N != 1 {
		t.Fatalf("%v %+v", err, res.Items)
	}
	before := me.git("rev-parse", "dev")
	other.commit("theirs.txt", "feat: theirs")
	other.git("push", "-q", "origin", "dev")
	res, _ = Run(Options{Root: me.dir})
	if res.Items[0].State != Diverged || me.git("rev-parse", "dev") != before {
		t.Fatalf("%+v", res.Items)
	}
}

func TestSyncBlockedByDirtyTree(t *testing.T) {
	me, other := world(t)
	other.commit("b.txt", "feat: b")
	other.git("push", "-q", "origin", "dev")
	os.WriteFile(filepath.Join(me.dir, "a.txt"), []byte("dirty\n"), 0o644)
	before := me.git("rev-parse", "dev")
	res, err := Run(Options{Root: me.dir})
	if err != nil || res.Items[0].State != Blocked || me.git("rev-parse", "dev") != before {
		t.Fatalf("%v %+v", err, res.Items)
	}
}

func TestSyncDryChangesNothing(t *testing.T) {
	me, other := world(t)
	other.commit("b.txt", "feat: b")
	other.git("push", "-q", "origin", "dev")
	before := me.git("rev-parse", "dev")
	known := me.git("rev-parse", "origin/dev")
	res, err := Run(Options{Root: me.dir, Dry: true})
	if err != nil || me.git("rev-parse", "dev") != before || me.git("rev-parse", "origin/dev") != known {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(strings.Join(res.Lines, "\n"), "без fetch") {
		t.Error("dry предупреждает, что fetch не делался")
	}
}

func TestSyncNoRemote(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := Run(Options{Root: dir}); err == nil || !strings.Contains(err.Error(), "нет удалённого") {
		t.Fatalf("%v", err)
	}
}

func TestShared(t *testing.T) {
	for n, want := range map[string]bool{"main": true, "dev": true, "build/1125-1020": true, "api/1125-1020": true, "wip/1125-1005/x": false, "topic": false, "a/b": false} {
		if Shared(n) != want {
			t.Errorf("%s: %v", n, !want)
		}
	}
}
