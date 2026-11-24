package commit

import (
	"strings"
	"testing"
)

func TestCheckMessage(t *testing.T) {
	for _, ok := range []string{"fix: x", "feat: " + strings.Repeat("я", 72)} {
		if err := CheckMessage(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "fix:x", "Fix: x", "fix:  ", "oops: x", "feat: " + strings.Repeat("x", 73)} {
		if CheckMessage(bad) == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
}

func TestOptionsSet(t *testing.T) {
	var o Options
	if err := o.Set("new"); err != nil || !o.New {
		t.Fatalf("%+v %v", o, err)
	}
	if err := o.Set("all"); err == nil {
		t.Error("неизвестный флаг должен отклоняться")
	}
}
