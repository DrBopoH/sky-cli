package readme

import (
	"strings"
	"testing"
)

func TestFiles(t *testing.T) {
	files := Files([]Entry{{Usage: "sky one", Summary: "первая"}, {Usage: "sky two", Summary: "вторая"}}, []Entry{{Usage: "op", Summary: "делает"}}, "scope = [x/]\n")
	if len(files) != 3 || files[0].Name != "readme.md" || files[1].Name != "git-versioning.md" || files[2].Name != "MT-unify-systems.md" {
		t.Fatalf("набор файлов: %v", files)
	}
	r := string(files[0].Content)
	for _, want := range []string{"- `sky one` — первая", "- `sky two` — вторая", "- `op` — делает", "## Патчи", "## Операции", "scope = [x/]", "(git-versioning.md)", "Как вести clouds/"} {
		if !strings.Contains(r, want) {
			t.Errorf("в readme.md нет %q", want)
		}
	}
	for _, f := range files {
		if strings.Contains(string(f.Content), "{{") {
			t.Errorf("%s: остался маркер подстановки", f.Name)
		}
	}
}

func TestStaticNoProjectContext(t *testing.T) {
	for _, f := range Files(nil, nil, "") {
		for _, bad := range []string{"clc.sh", "Док", "Ζωή", "zoe"} {
			if strings.Contains(string(f.Content), bad) {
				t.Errorf("%s: контекст проекта «%s» не должен попадать в статику", f.Name, bad)
			}
		}
	}
}
