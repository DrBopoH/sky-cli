package apply

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func git(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ignored сообщает, лежит ли путь под .gitignore (git check-ignore: 0 — да, 1 — нет).
func ignored(root, p string) (bool, error) {
	err := exec.Command("git", "-C", root, "check-ignore", "-q", "--", p).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git check-ignore %s: %v", p, err)
}

// commitPaths делает один коммит из перечисленных путей (записанных и удалённых). Пути из .gitignore пропускаются.
// Возвращает короткий хэш; пустой, если коммитить нечего.
func commitPaths(root, msg string, paths []string) (hash string, skipped []string, err error) {
	var take []string
	for _, p := range paths {
		ig, err := ignored(root, p)
		if err != nil {
			return "", nil, err
		}
		if ig {
			skipped = append(skipped, p)
			continue
		}
		take = append(take, p)
	}
	if len(take) == 0 {
		return "", skipped, nil
	}
	if _, err := git(root, append([]string{"add", "-A", "--"}, take...)...); err != nil {
		return "", skipped, err
	}
	if _, err := git(root, append([]string{"commit", "-q", "-m", msg, "--"}, take...)...); err != nil {
		return "", skipped, err
	}
	hash, err = git(root, "rev-parse", "--short=7", "HEAD")
	return hash, skipped, err
}

// unstage возвращает индекс путей к HEAD после неудачного коммита; ошибки не важны.
// В репозитории без коммитов reset не работает, там индекс просто очищается от этих путей.
func unstage(root string, paths []string) {
	if len(paths) == 0 {
		return
	}
	if exec.Command("git", append([]string{"-C", root, "reset", "-q", "--"}, paths...)...).Run() == nil {
		return
	}
	exec.Command("git", append([]string{"-C", root, "rm", "--cached", "-r", "-q", "--ignore-unmatch", "--"}, paths...)...).Run()
}
