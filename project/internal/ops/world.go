package ops

import (
	"github.com/DrBopoH/sky-cli/internal/guard"
	"os"
	"path/filepath"
)

// world — вид репозитория для операций.
type world struct{ root string }

func (w world) Guard(p string) error { return guard.Path(p) }

func (w world) Stat(p string) (bool, bool) {
	fi, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p)))
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}
