package apply

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/notation"
)

const (
	StCreated  = "created"
	StModified = "modified"
	StDeleted  = "deleted"
)

type change struct {
	Status string
	Path   string
	Old    []byte // прежнее содержимое для modified и deleted
	OldExe bool
	Effect Effect
}

var nameSafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// nextStageDir создаёт .sky/stages/NNNN-<время>-<имя патча>/.
func nextStageDir(stages, patchName string) (string, error) {
	entries, _ := os.ReadDir(stages)
	n := 0
	for _, e := range entries {
		var k int
		if _, err := fmt.Sscanf(e.Name(), "%04d-", &k); err == nil && k > n {
			n = k
		}
	}
	base := strings.TrimSuffix(filepath.Base(patchName), filepath.Ext(patchName))
	dir := filepath.Join(stages, fmt.Sprintf("%04d-%s-%s", n+1, time.Now().Format("20060102-150405"), nameSafe.ReplaceAllString(base, "_")))
	return dir, os.MkdirAll(dir, 0o755)
}

// classify сверяет эффекты с диском: что создаётся, меняется, удаляется; неизменные записи отбрасывает.
func classify(root string, effects []Effect) (changes []change, same []string, err error) {
	for _, e := range effects {
		abs, err := resolve(root, e.Path)
		if err != nil {
			return nil, nil, err
		}
		fi, serr := os.Lstat(abs)
		exists := serr == nil
		if exists && fi.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("%s: на месте файла символическая ссылка", e.Path)
		}
		if exists && fi.IsDir() {
			return nil, nil, fmt.Errorf("%s: на месте файла папка", e.Path)
		}
		var old []byte
		if exists {
			if old, err = os.ReadFile(abs); err != nil {
				return nil, nil, err
			}
		}
		switch e.Kind {
		case Write:
			switch {
			case !exists:
				changes = append(changes, change{Status: StCreated, Path: e.Path, Effect: e})
			case bytes.Equal(old, e.Content) && (fi.Mode()&0o111 != 0) == e.Exec:
				same = append(same, e.Path)
			default:
				changes = append(changes, change{Status: StModified, Path: e.Path, Old: old, OldExe: fi.Mode()&0o111 != 0, Effect: e})
			}
		case Delete:
			if exists {
				changes = append(changes, change{Status: StDeleted, Path: e.Path, Old: old, OldExe: fi.Mode()&0o111 != 0, Effect: e})
			}
		}
	}
	return changes, same, nil
}

// saveBefore пишет снимок «до» и manifest.txt в папку записи.
func saveBefore(dir, patchName string, changes []change) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var man strings.Builder
	fmt.Fprintf(&man, "# patch: %s\n", filepath.Base(patchName))
	for _, c := range changes {
		fmt.Fprintf(&man, "%s\t%s\n", c.Status, c.Path)
		if c.Status == StCreated {
			continue
		}
		h := &zip.FileHeader{Name: c.Path, Method: zip.Deflate}
		mode := os.FileMode(0o644)
		if c.OldExe {
			mode = 0o755
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := w.Write(c.Old); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "before.zip"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(man.String()), 0o644)
}

func writeFile(abs string, content []byte, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".sky-tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), abs)
}

// pruneEmpty убирает опустевшие каталоги вверх до корня; внутри .sky/ каталоги не трогает.
func pruneEmpty(root, abs string) {
	dir := filepath.Dir(abs)
	for dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)) {
		rel, _ := filepath.Rel(root, dir)
		if rel == ".sky" || strings.HasPrefix(filepath.ToSlash(rel), ".sky/") {
			return
		}
		if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func execute(root string, changes []change) error {
	for _, c := range changes {
		abs, err := resolve(root, c.Path)
		if err != nil {
			return err
		}
		switch c.Effect.Kind {
		case Write:
			err = writeFile(abs, c.Effect.Content, c.Effect.Exec)
		case Delete:
			if err = os.Remove(abs); err == nil {
				pruneEmpty(root, abs)
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", c.Path, err)
		}
	}
	return nil
}

// Restore возвращает файлы в состояние «до» по записи в stages/.
func Restore(root, stageDir string) error {
	man, err := os.ReadFile(filepath.Join(stageDir, "manifest.txt"))
	if err != nil {
		return err
	}
	recs, err := notation.Parse(string(man), 2)
	if err != nil {
		return err
	}
	zr, err := zip.OpenReader(filepath.Join(stageDir, "before.zip"))
	if err != nil {
		return err
	}
	defer zr.Close()
	old := map[string]*zip.File{}
	for _, f := range zr.File {
		old[f.Name] = f
	}
	var errs []error
	for _, r := range recs {
		status, p := r[0], r[1]
		abs, err := resolve(root, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if status == StCreated {
			if fi, err := os.Lstat(abs); err == nil && !fi.IsDir() {
				if err := os.Remove(abs); err != nil {
					errs = append(errs, err)
				}
				pruneEmpty(root, abs)
			}
			continue
		}
		f, ok := old[p]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: нет в снимке «до»", p))
			continue
		}
		rc, err := f.Open()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err == nil {
			err = writeFile(abs, b, f.Mode()&0o111 != 0)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}
