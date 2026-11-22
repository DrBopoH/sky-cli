package ops

import (
	"reflect"
	"strings"
	"testing"
)

type fakeWorld struct {
	files map[string]bool // путь → папка ли
	bad   string
}

func (w fakeWorld) Guard(p string) error {
	if p == w.bad {
		return errBad
	}
	return nil
}
func (w fakeWorld) Stat(p string) (bool, bool) { d, ok := w.files[p]; return ok, d }

type badErr struct{}

func (badErr) Error() string { return "запрещённый путь" }

var errBad = badErr{}

func TestParse(t *testing.T) {
	got, err := Parse("# c\n\na b/c.txt\r\n\nx\n", 1)
	if err != nil || !reflect.DeepEqual(got, [][]string{{"a b/c.txt"}, {"x"}}) {
		t.Fatalf("%v %v", got, err)
	}
	got, err = Parse("a\tb\n", 2)
	if err != nil || !reflect.DeepEqual(got, [][]string{{"a", "b"}}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"a\tb\n", "\ta\n"} {
		if _, err := Parse(bad, 1); err == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
	if _, err := Parse("a\n", 2); err == nil || !strings.Contains(err.Error(), "строка 1") {
		t.Errorf("номер строки в ошибке: %v", err)
	}
}

func TestDeletePlan(t *testing.T) {
	op, ok := Lookup("delete")
	if !ok || op.Fields != 1 || op.Doc == "" {
		t.Fatal("реестр")
	}
	w := fakeWorld{files: map[string]bool{"a.txt": false, "dir": true}, bad: "secret.db"}
	r, err := op.Plan([]string{"a.txt"}, w)
	if err != nil || len(r.Effects) != 1 || r.Effects[0].Kind != Delete || r.Effects[0].Path != "a.txt" {
		t.Fatalf("%v %v", r, err)
	}
	r, err = op.Plan([]string{"gone.txt"}, w)
	if err != nil || len(r.Effects) != 0 || len(r.Notes) != 1 {
		t.Fatalf("пропуск: %v %v", r, err)
	}
	if _, err := op.Plan([]string{"dir"}, w); err == nil {
		t.Error("папка должна отклоняться")
	}
	if _, err := op.Plan([]string{"secret.db"}, w); err == nil {
		t.Error("guard должен отклонять")
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("rm-rf"); ok {
		t.Fatal("неизвестной операции быть не должно")
	}
	if len(All()) == 0 {
		t.Fatal("пустой реестр")
	}
}

func TestCommitOp(t *testing.T) {
	op, ok := Lookup("commit")
	if !ok {
		t.Fatal("операции commit нет в реестре")
	}
	res, err := op.Plan([]string{"fix: something"}, nil)
	if err != nil || res.Commit != "fix: something" || len(res.Effects) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	for _, bad := range []string{"", "fix:x", "Fix: x", "fix:  ", "oops: x", "feat: " + strings.Repeat("x", 73)} {
		if _, err := op.Plan([]string{bad}, nil); err == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
}
