package license

import (
	"strings"
	"testing"
)

func TestRenderAll(t *testing.T) {
	f := Fields{Year: "2030", Holder: "Acme Ltd", Email: "a@acme.test"}
	for _, id := range Known() {
		out, err := Render(id, f)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if strings.Contains(out, "{{") {
			t.Errorf("%s: остался маркер", id)
		}
		if id != "Unlicense" && !strings.Contains(out, "2030") {
			t.Errorf("%s: нет года", id)
		}
		if id != "Unlicense" && !strings.Contains(out, "Acme Ltd <a@acme.test>") {
			t.Errorf("%s: нет holder и email", id)
		}
		if !strings.HasSuffix(out, "\n") {
			t.Errorf("%s: нет перевода строки в конце", id)
		}
	}
}

func TestKnownList(t *testing.T) {
	got := strings.Join(Known(), ",")
	if got != "BSD-3-Clause,ISC,MIT,Unlicense" {
		t.Fatal(got)
	}
}

func TestMITCopyrightLine(t *testing.T) {
	out, _ := Render("MIT", Fields{Year: "2030", Holder: "A"})
	if !strings.HasPrefix(out, "MIT License\n\nCopyright (c) 2030 A\n\nPermission is hereby granted") {
		t.Errorf("%q", out[:80])
	}
}

func TestCheck(t *testing.T) {
	if err := Check("GPL", Fields{}); err == nil || !strings.Contains(err.Error(), "MIT") {
		t.Errorf("неизвестный тип: %v", err)
	}
	if err := Check("MIT", Fields{}); err == nil {
		t.Error("holder обязателен для MIT")
	}
	if err := Check("Unlicense", Fields{}); err != nil {
		t.Errorf("Unlicense без holder: %v", err)
	}
}
