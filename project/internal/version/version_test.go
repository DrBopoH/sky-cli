package version

import "testing"

func TestStringUsesInjectedVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "v1.a1.2+2.abc1234"
	if got := String(); got != "v1.a1.2+2.abc1234" {
		t.Errorf("подставленная версия потеряна: %q", got)
	}
}

func TestEmptyVersionIsDev(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = ""
	if got := String(); got == "" || got[:3] != "dev" {
		t.Errorf("пустая версия должна быть dev…, получено %q", got)
	}
}
