package pr

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
	log string
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
	os.WriteFile(filepath.Join(r.dir, name), []byte(name+"\n"), 0o644)
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// newRepo — dev с коммитом, origin (bare) с dev, ветка фичи с одним коммитом, подставной gh в PATH.
// existing — что подставной gh ответит на «pr list» (адрес уже открытого PR).
func newRepo(t *testing.T, existing string) repo {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	r := repo{t: t, dir: filepath.Join(base, "work"), log: filepath.Join(base, "gh.log")}
	os.MkdirAll(r.dir, 0o755)
	r.git("init", "-q", "-b", "dev")
	r.git("config", "user.name", "t")
	r.git("config", "user.email", "t@t")
	r.git("config", "commit.gpgsign", "false")
	r.commit("a.txt", "chore: init")
	r.git("remote", "add", "origin", bare)
	r.git("push", "-q", "origin", "dev")
	r.git("fetch", "-q", "origin")
	r.git("checkout", "-q", "-b", "wip/1125-1005/x")
	r.commit("b.txt", "feat: add b")
	gh := "#!/bin/sh\necho \"$@\" >> \"" + r.log + "\"\ncase \"$1 $2\" in\n\"pr list\") printf '%s' \"" + existing + "\";;\n\"pr create\") echo \"https://github.com/o/r/pull/7\";;\nesac\n"
	bin := filepath.Join(base, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return r
}

func (r repo) opts() Options {
	return Options{Root: r.dir, Branch: "wip/1125-1005/x", Summary: "Add b", Kind: "module", Module: "storage", Chain: []string{"api", "storage"}}
}

func (r repo) ghLog() string {
	b, _ := os.ReadFile(r.log)
	return string(b)
}

func TestPrPushesBranchAndOpensPR(t *testing.T) {
	r := newRepo(t, "")
	res, err := Run(r.opts())
	if err != nil || !res.Done || res.URL != "https://github.com/o/r/pull/7" {
		t.Fatalf("%v %+v", err, res)
	}
	if got := r.git("ls-remote", "--heads", "origin", "wip/1125-1005/x"); !strings.Contains(got, "refs/heads/wip/1125-1005/x") {
		t.Errorf("ветка не запушена: %q", got)
	}
	log := r.ghLog()
	for _, want := range []string{"pr create --base dev --head wip/1125-1005/x --title Add b --body Add b", "sky-target: module", "sky-module: storage", "sky-chain: api,storage"} {
		if !strings.Contains(log, want) {
			t.Errorf("в вызове gh нет %q:\n%s", want, log)
		}
	}
	if r.git("config", "--get", "branch.wip/1125-1005/x.sky-pr") != res.URL {
		t.Error("адрес PR не записан в настройки ветки")
	}
}

func TestPrSeedBodyAndDraft(t *testing.T) {
	r := newRepo(t, "")
	o := r.opts()
	o.Kind, o.Module, o.Chain, o.Draft = "seed", "", nil, true
	if _, err := Run(o); err != nil {
		t.Fatal(err)
	}
	log := r.ghLog()
	if !strings.Contains(log, "sky-target: seed") || strings.Contains(log, "sky-module") || !strings.Contains(log, "--draft") {
		t.Errorf("%s", log)
	}
}

func TestPrFixGoesIntoBuildBranch(t *testing.T) {
	r := newRepo(t, "")
	r.git("branch", "build/1125-1020", "dev")
	r.git("push", "-q", "origin", "build/1125-1020")
	r.git("fetch", "-q", "origin")
	o := r.opts()
	o.Kind, o.Module, o.Chain, o.Base = "fix", "", nil, "build/1125-1020"
	res, err := Run(o)
	if err != nil || !res.Done {
		t.Fatalf("%v %+v", err, res)
	}
	log := r.ghLog()
	if !strings.Contains(log, "--base build/1125-1020") || !strings.Contains(log, "sky-target: fix") || !strings.Contains(log, "sky-build: build/1125-1020") {
		t.Errorf("%s", log)
	}
}

func TestPrExistingOnlyPushes(t *testing.T) {
	r := newRepo(t, "https://github.com/o/r/pull/3")
	r.commit("c.txt", "feat: more")
	res, err := Run(r.opts())
	if err != nil || res.URL != "https://github.com/o/r/pull/3" {
		t.Fatalf("%v %+v", err, res)
	}
	if strings.Contains(r.ghLog(), "pr create") {
		t.Error("второй PR открывать нельзя")
	}
	if r.git("rev-parse", "origin/wip/1125-1005/x") != r.git("rev-parse", "HEAD") {
		t.Error("новые коммиты должны уйти в удалённую ветку")
	}
}

func TestPrDryChangesNothing(t *testing.T) {
	r := newRepo(t, "")
	o := r.opts()
	o.Dry = true
	res, err := Run(o)
	if err != nil || res.Done || len(res.Lines) == 0 {
		t.Fatalf("%v %+v", err, res)
	}
	if r.git("ls-remote", "--heads", "origin", "wip/*") != "" || strings.Contains(r.ghLog(), "pr create") {
		t.Error("dry ничего не пушит и не открывает")
	}
}

func TestPrRefusals(t *testing.T) {
	r := newRepo(t, "")
	cases := []struct {
		name string
		mod  func(*Options)
		want string
	}{
		{"fix без сборки", func(o *Options) { o.Kind = "fix" }, "ветка сборки"},
		{"без цели", func(o *Options) { o.Kind = "" }, "sky feature"},
		{"пустое резюме", func(o *Options) { o.Summary = " " }, "нужно резюме"},
		{"длинное", func(o *Options) { o.Summary = strings.Repeat("я", 73) }, "длиннее"},
		{"нет базы", func(o *Options) { o.Base = "nope" }, "нет ветки nope"},
	}
	for _, c := range cases {
		o := r.opts()
		c.mod(&o)
		if _, err := Run(o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if r.git("ls-remote", "--heads", "origin", "wip/*") != "" {
		t.Error("при отказе ничего не пушится")
	}
}

func TestPrNoRemoteNoCommitsDirty(t *testing.T) {
	r := newRepo(t, "")
	os.WriteFile(filepath.Join(r.dir, "b.txt"), []byte("changed\n"), 0o644)
	if _, err := Run(r.opts()); err == nil || !strings.Contains(err.Error(), "незакоммиченные") {
		t.Fatalf("грязное дерево: %v", err)
	}
	r.git("checkout", "-q", "--", "b.txt")
	r.git("checkout", "-q", "dev")
	r.git("branch", "wip/1125-1006/empty")
	o := r.opts()
	o.Branch = "wip/1125-1006/empty"
	if _, err := Run(o); err == nil || !strings.Contains(err.Error(), "нет коммитов") {
		t.Fatalf("пустая ветка: %v", err)
	}
	r.git("remote", "remove", "origin")
	if _, err := Run(r.opts()); err == nil || !strings.Contains(err.Error(), "нет удалённого") {
		t.Fatalf("нет origin: %v", err)
	}
}

func TestPrNoGh(t *testing.T) {
	r := newRepo(t, "")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	only := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(only, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", only) // git есть, gh нет
	if _, err := Run(r.opts()); err == nil || !strings.Contains(err.Error(), "gh") {
		t.Fatalf("%v", err)
	}
}

func TestBodyShape(t *testing.T) {
	b := Body(Options{Branch: "wip/x", Summary: "S", Kind: "seed"})
	if b != "S\n\n---\nsky-branch: wip/x\nsky-target: seed\n" {
		t.Errorf("%q", b)
	}
}

func TestParseBodyRoundTrip(t *testing.T) {
	for _, o := range []Options{
		{Branch: "wip/x", Summary: "S", Kind: "seed"},
		{Branch: "wip/x", Summary: "S", Kind: "module", Module: "storage", Chain: []string{"api", "storage"}},
		{Branch: "wip/x", Summary: "S", Kind: "fix", Base: "build/1125-1020"},
	} {
		got, ok := ParseBody(Body(o))
		want := Target{Branch: o.Branch, Kind: o.Kind, Module: o.Module, Chain: o.Chain}
		if o.Kind == "fix" {
			want.Build = o.Base
		}
		if !ok || got.Branch != want.Branch || got.Kind != want.Kind || got.Module != want.Module || strings.Join(got.Chain, ",") != strings.Join(want.Chain, ",") || got.Build != want.Build {
			t.Errorf("%+v → %+v (%v)", o, got, ok)
		}
	}
	if _, ok := ParseBody("просто текст\n\nsky-branch: x"); ok {
		t.Error("без sky-target цели нет")
	}
}
