package work

import "testing"

func TestRestorable(t *testing.T) {
	for p, want := range map[string]bool{
		"main.go": true, ".sky/sky.conf": false, ".sky/foryou/archive.md": false, ".sky/readme/readme.md": false, ".sky/clouds/n.md": true,
	} {
		if restorable(p) != want {
			t.Errorf("restorable(%q) = %v", p, !want)
		}
	}
}
