package conf

import (
	"reflect"
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

func TestSectionsAreRejected(t *testing.T) {
	for _, src := range []string{"[license]\ntype = MIT\n", "[backend/]\ntest = go test\n"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("секции не поддерживаются: %q", src)
		}
	}
}
