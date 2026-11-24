// Package gitx — единственная точка доступа sky к git: все вызовы процесса git идут отсюда.
// Остальные пакеты git не запускают, а зовут функции этого.
package gitx

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func cmd(root string, args []string) *exec.Cmd {
	return exec.Command("git", append([]string{"-C", root}, args...)...)
}

// Run выполняет git; возвращает объединённый вывод без крайних пробелов. Ошибка несёт вывод git.
func Run(root string, args ...string) (string, error) {
	out, err := cmd(root, args).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Try возвращает stdout git без хвостовых переводов строки; при ошибке — пустую строку.
func Try(root string, args ...string) string {
	out, err := cmd(root, args).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// OK сообщает, завершился ли git успешно.
func OK(root string, args ...string) bool {
	return cmd(root, args).Run() == nil
}

// Z разбирает stdout git с нулевыми разделителями (-z) в список непустых строк.
func Z(root string, args ...string) ([]string, error) {
	out, err := cmd(root, args).Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v", args[0], err)
	}
	var r []string
	for _, s := range strings.Split(string(out), "\x00") {
		if s != "" {
			r = append(r, s)
		}
	}
	return r, nil
}

// Toplevel возвращает корень репозитория, в котором лежит dir.
func Toplevel(dir string) (string, error) {
	out, err := cmd(dir, []string{"rev-parse", "--show-toplevel"}).Output()
	if err != nil {
		return "", errors.New("это не git-репозиторий")
	}
	return strings.TrimSpace(string(out)), nil
}

// Init создаёт репозиторий в dir с веткой main (старый git без -b: init и symbolic-ref).
func Init(dir string) error {
	if out, e := cmd(dir, []string{"init", "-q", "-b", "main"}).CombinedOutput(); e != nil {
		if out2, e2 := cmd(dir, []string{"init", "-q"}).CombinedOutput(); e2 != nil {
			return fmt.Errorf("git init не удался: %s%s", out, out2)
		}
		if out, e := cmd(dir, []string{"symbolic-ref", "HEAD", "refs/heads/main"}).CombinedOutput(); e != nil {
			return fmt.Errorf("не выбрать ветку main: %s", out)
		}
	}
	return nil
}

// Head — короткий (7 знаков) хэш HEAD; пустая строка, если коммитов нет.
func Head(root string) string {
	return strings.TrimSpace(Try(root, "rev-parse", "--short=7", "HEAD"))
}

// HasCommits сообщает, есть ли в репозитории хотя бы один коммит.
func HasCommits(root string) bool {
	return OK(root, "rev-parse", "--verify", "-q", "HEAD")
}

// CanCommit сообщает, знает ли git имя и почту автора для коммита в dir.
func CanCommit(dir string) bool {
	return OK(dir, "var", "GIT_COMMITTER_IDENT")
}

// Ignored сообщает, лежит ли путь под .gitignore (git check-ignore: 0 — да, 1 — нет).
func Ignored(root, p string) (bool, error) {
	err := cmd(root, []string{"check-ignore", "-q", "--", p}).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git check-ignore %s: %v", p, err)
}

// Blob возвращает содержимое файла в HEAD и признак исполняемого; ok=false, если файла в HEAD нет.
func Blob(root, p string) (data []byte, exe, ok bool) {
	tree, err := cmd(root, []string{"ls-tree", "HEAD", "--", p}).Output()
	if err != nil || len(tree) == 0 {
		return nil, false, false
	}
	exe = strings.HasPrefix(string(tree), "100755")
	data, err = cmd(root, []string{"cat-file", "blob", "HEAD:" + p}).Output()
	return data, exe, err == nil
}

// Untracked — новые файлы дерева, не закрытые .gitignore.
func Untracked(root string) ([]string, error) {
	return Z(root, "ls-files", "-z", "--others", "--exclude-standard")
}

// Visible — файлы репозитория по версии git: отслеживаемые и новые без игнорируемых.
func Visible(root string) ([]string, error) {
	return Z(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate")
}

// Dirty — изменения отслеживаемых файлов (status --porcelain без новых); пусто, если дерево чистое.
func Dirty(root string) string {
	return strings.TrimSpace(Try(root, "status", "--porcelain", "--untracked-files=no"))
}

// Unstage возвращает индекс путей к HEAD; ошибки не важны. В репозитории без коммитов
// reset не работает, там индекс просто очищается от этих путей.
func Unstage(root string, paths []string) {
	if len(paths) == 0 {
		return
	}
	if OK(root, append([]string{"reset", "-q", "--"}, paths...)...) {
		return
	}
	OK(root, append([]string{"rm", "--cached", "-r", "-q", "--ignore-unmatch", "--"}, paths...)...)
}
