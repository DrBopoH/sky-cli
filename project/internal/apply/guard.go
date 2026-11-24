// Package apply применяет патч: читает архив, проверяет пути, снимает «до», пишет эффекты и при ошибке откатывает.
package apply

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var secretRe = regexp.MustCompile(`\.(db|sqlite|sqlite3|env|pem|key)(-wal|-shm)?$`)

// IsSensitive сообщает, похоже ли имя на базу или ключ (по расширению).
func IsSensitive(p string) bool { return secretRe.MatchString(p) }

// GuardPath — единственная проверка путей патча: и файлов, и записей операций.
func GuardPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("пустой путь")
	case strings.HasPrefix(p, "/"):
		return fmt.Errorf("%s: путь должен быть от корня репозитория", p)
	case strings.ContainsAny(p, "\\\x00"):
		return fmt.Errorf("%s: недопустимый символ в пути", p)
	}
	for _, r := range p {
		if r < 0x20 {
			return fmt.Errorf("%q: управляющий символ в пути", p)
		}
	}
	if path.Clean(p) != p {
		return fmt.Errorf("%s: путь должен быть нормализованным, без «..», «.» и «//»", p)
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		switch s {
		case "..":
			return fmt.Errorf("%s: путь выходит за репозиторий", p)
		case ".git":
			return fmt.Errorf("%s: .git трогать нельзя", p)
		}
	}
	if segs[0] == ".sky" {
		if len(segs) < 3 || segs[1] != "clouds" {
			return fmt.Errorf("%s: в .sky/ патчем меняется только clouds/", p)
		}
	}
	if secretRe.MatchString(p) {
		return fmt.Errorf("%s: базы и ключи в патч не принимаются", p)
	}
	return nil
}

// resolve возвращает абсолютный путь внутри корня и проверяет, что ни один каталог по дороге не ведёт за его пределы.
func resolve(root, rel string) (string, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: путь ведёт за пределы репозитория", rel)
	}
	return abs, nil
}

// world — вид репозитория для операций.
type world struct{ root string }

func (w world) Guard(p string) error { return GuardPath(p) }

func (w world) Stat(p string) (bool, bool) {
	fi, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p)))
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}
