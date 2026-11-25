package named

import "testing"

func TestParseAndMessage(t *testing.T) {
	_, v, ok := Parse("Add x | v1.1.29+29.def5678")
	if !ok || !v.Full || v.R != 1 || v.B != 1 || v.C != 29 || v.T != 29 || v.Hash != "def5678" {
		t.Fatalf("full: %+v %v", v, ok)
	}
	s, v, ok := Parse("Add x | v28.abc1234")
	if !ok || v.Full || v.C != 28 || s != "Add x" {
		t.Fatalf("branch: %q %+v %v", s, v, ok)
	}
	if got := Message("Add x", v); got != "Add x | v28.abc1234" {
		t.Fatalf("message %q", got)
	}
	for _, bad := range []string{"feat: x", "x | v1.1.29.def5678", "x | v28.ABC1234", "x | v28.abc123"} {
		if _, _, ok := Parse(bad); ok {
			t.Fatalf("«%s» не именованный", bad)
		}
	}
}

func TestParseBase(t *testing.T) {
	v, err := ParseBase("1.1.35+35")
	if err != nil || v.C != 35 || v.T != 35 || !v.Full {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := ParseBase("1.1.35"); err == nil {
		t.Fatal("без +T отказ")
	}
}
