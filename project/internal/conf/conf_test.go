package conf

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseScope(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"project"}, []string{"project/"}},
		{[]string{"backend/", "frontend/"}, []string{"backend/", "frontend/"}},
		{[]string{"[backend/, frontend/]"}, []string{"backend/", "frontend/"}},
		{[]string{"[backend/,", "frontend/]"}, []string{"backend/", "frontend/"}},
		{[]string{"a/b//", "a/b"}, []string{"a/b/"}},
		{[]string{`"x"`}, []string{"x/"}},
		{[]string{"."}, []string{"./"}},
	}
	for _, c := range cases {
		got, err := ParseScope(c.in)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v, %v want %v", c.in, got, err, c.want)
		}
	}
}

func TestParseScopeRejects(t *testing.T) {
	for _, in := range []string{"", "[]", "/abs", "~/x", "../x", "..", ".sky", ".git/x", ".sky/clouds"} {
		if got, err := ParseScope([]string{in}); err == nil {
			t.Errorf("%q: ждали ошибку, получили %v", in, got)
		}
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	c := Conf{Scope: []string{"backend/", "frontend/"}, Inbox: "/tmp/in", Regulation: "https://example.com/reg.md"}
	got, err := Parse(Render(c))
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("got %v, %v", got, err)
	}
	d, err := Parse(Render(Default()))
	if err != nil || !reflect.DeepEqual(d, Default()) {
		t.Fatalf("default: %v, %v", d, err)
	}
}

func TestParseDefaultsAndErrors(t *testing.T) {
	c, err := Parse("# только комментарий\n")
	if err != nil || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("пустой конфиг: %v, %v", c, err)
	}
	for _, bad := range []string{"scope\n", "color = red\n", "scope = [../x]\n", "inbox =\n", "regulation = ftp://x\n"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
}

func TestCommandsSections(t *testing.T) {
	src := "scope = [backend/, frontend/]\n\n[backend/]\ntest = go test ./...\n\n[frontend/]\ntest = bun run test\nbuild = bun run build\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if c.Cmd["backend/"].Test != "go test ./..." || c.Cmd["backend/"].Build != "" || c.Cmd["frontend/"].Build != "bun run build" {
		t.Fatalf("%v", c.Cmd)
	}
	if c.Cmd["frontend/"].Get("test") != "bun run test" || c.Cmd["frontend/"].Get("nope") != "" {
		t.Error("Get")
	}
	back, err := Parse(Render(c))
	if err != nil || !reflect.DeepEqual(back, c) {
		t.Fatalf("круговой обмен: %v %v", back, err)
	}
}

func TestCommandsSectionErrors(t *testing.T) {
	for _, bad := range []string{
		"scope = [a/]\n[b/]\ntest = x\n",         // папки нет в scope
		"scope = [a/]\n[a]\ntest = x\n",          // секция без слэша
		"scope = [a/, b/]\n[a/, b/]\ntest = x\n", // список вместо папки
		"scope = [a/]\n[a/]\ncolor = red\n",      // неизвестный ключ секции
		"scope = [a/]\n[a/]\ntest =\n",           // пустая команда
		"scope = [a/]\n[a/]\nscope = [b/]\n",     // глобальный ключ внутри секции
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
}

func TestRenderKeepsCommandsOnlyForScope(t *testing.T) {
	c := Conf{Scope: []string{"a/"}, Inbox: "~/x", Cmd: map[string]Commands{"a/": {Test: "t"}, "gone/": {Test: "x"}}}
	out := Render(c)
	if !strings.Contains(out, "[a/]") || strings.Contains(out, "gone/") {
		t.Fatalf("%s", out)
	}
}

func TestLicenseSectionIsGone(t *testing.T) {
	if _, err := Parse("[license]\ntype = MIT\n"); err == nil {
		t.Error("секции без слэша зарезервированы, [license] больше не принимается")
	}
}
