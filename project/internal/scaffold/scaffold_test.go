package scaffold

import (
	"github.com/DrBopoH/sky-cli/internal/conf"
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
	r := Readme("demo", []string{"backend/", "frontend/"}, nil)
	for _, want := range []string{"# demo", "- `backend/` — исходный код", "- `frontend/` — исходный код"} {
		if !strings.Contains(r, want) {
			t.Errorf("в README нет %q:\n%s", want, r)
		}
	}
	if strings.Contains(r, "{{") {
		t.Error("остался маркер")
	}
	if strings.Contains(r, "Лицензия") {
		t.Error("без [license] раздела о лицензии быть не должно")
	}
}

func TestReadmeLicense(t *testing.T) {
	if r := Readme("d", []string{"p/"}, &conf.License{Type: "MIT", Holder: "X"}); !strings.Contains(r, "## Лицензия\n\nMIT, см. [LICENSE](LICENSE).") {
		t.Errorf("type:\n%s", r)
	}
	if r := Readme("d", []string{"p/"}, &conf.License{URL: "https://e.org/l"}); !strings.Contains(r, "[Условия лицензии](https://e.org/l)") {
		t.Errorf("url:\n%s", r)
	}
}
