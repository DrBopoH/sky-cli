package scaffold

import (
	"strings"
	"testing"
)

func TestGitignoreHasEssentials(t *testing.T) {
	g := Gitignore()
	for _, want := range []string{".sky/", ".env", "*.db", "*.sublime-workspace", "node_modules/", ".DS_Store", "!.env.example"} {
		if !strings.Contains(g, want) {
			t.Errorf("в шаблоне .gitignore нет %q", want)
		}
	}
}

func TestReadme(t *testing.T) {
	r := Readme("demo", []string{"backend/", "frontend/"})
	for _, want := range []string{"# demo", "- `backend/` — исходный код", "- `frontend/` — исходный код"} {
		if !strings.Contains(r, want) {
			t.Errorf("в README нет %q:\n%s", want, r)
		}
	}
	if strings.Contains(r, "{{") {
		t.Error("остался маркер")
	}
}
