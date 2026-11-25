package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGh — подставной gh: на «api …/rulesets» (GET) отвечает содержимым файла list, остальные вызовы и ввод пишет в лог.
func fakeGh(t *testing.T, list string) (root, log string) {
	t.Helper()
	base := t.TempDir()
	log = filepath.Join(base, "gh.log")
	listFile := filepath.Join(base, "list.json")
	os.WriteFile(listFile, []byte(list), 0o644)
	script := "#!/bin/sh\necho \"$@\" >> \"" + log + "\"\nif [ \"$2\" = \"-X\" ]; then cat >> \"" + log + "\"; echo >> \"" + log + "\"; else cat \"" + listFile + "\"; fi\n"
	bin := filepath.Join(base, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return base, log
}

func read(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProtectCreatesBothRules(t *testing.T) {
	root, log := fakeGh(t, "[]")
	res, err := Run(Options{Root: root})
	if err != nil || len(res.Lines) != 2 {
		t.Fatalf("%v %+v", err, res)
	}
	l := read(t, log)
	if strings.Count(l, "-X POST repos/:owner/:repo/rulesets") != 2 || strings.Contains(l, "PUT") {
		t.Errorf("оба правила создаются: %s", l)
	}
	for _, want := range []string{`"type": "deletion"`, `"type": "non_fast_forward"`, "refs/heads/build/**", "refs/heads/*/*"} {
		if !strings.Contains(l, want) {
			t.Errorf("в запросах нет %q", want)
		}
	}
}

func TestProtectUpdatesByNameWithoutDuplicates(t *testing.T) {
	root, log := fakeGh(t, `[{"id":11,"name":"sky regulation: no deletion"},{"id":12,"name":"чужое правило"}]`)
	if _, err := Run(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	l := read(t, log)
	if !strings.Contains(l, "-X PUT repos/:owner/:repo/rulesets/11") || strings.Count(l, "-X POST") != 1 || strings.Contains(l, "rulesets/12") {
		t.Errorf("существующее обновляется, недостающее создаётся, чужое не трогается:\n%s", l)
	}
}

func TestProtectDryTouchesNothing(t *testing.T) {
	root, log := fakeGh(t, "[]")
	res, err := Run(Options{Root: root, Dry: true})
	if err != nil || len(res.Lines) != 2 || !strings.Contains(res.Lines[0], "будет создано") {
		t.Fatalf("%v %+v", err, res)
	}
	if strings.Contains(read(t, log), "-X") {
		t.Error("dry ничего не отправляет на GitHub")
	}
}

func TestProtectNoGh(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Run(Options{Root: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "gh") {
		t.Fatalf("%v", err)
	}
}
