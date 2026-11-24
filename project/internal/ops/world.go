package ops

import (
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/apply"
)

// world — вид репозитория для операций.
type world struct{ root string }

func (w world) Guard(p string) error { return apply.GuardPath(p) }

func (w world) Stat(p string) (bool, bool) {
	fi, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p)))
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}
