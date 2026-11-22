package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/ops"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func commitCommand() Command {
	return Command{
		Name:      "commit",
		Usage:     `sky commit [--new] "тип: текст"`,
		Summary:   "закоммитить ручные правки одним коммитом: изменения отслеживаемых файлов; --new добавляет и новые файлы (кроме игнорируемых), без него sky их только перечислит; сообщение как у операции commit",
		NeedsInit: true,
		Run:       runCommit,
	}
}

func runCommit(a *App, env Env, args []string) error {
	withNew := false
	var rest []string
	for _, x := range args {
		if x == "--new" {
			withNew = true
		} else {
			rest = append(rest, x)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf(`сообщение одним аргументом в кавычках: sky commit [--new] "тип: текст"`)
	}
	msg := rest[0]
	if err := ops.CheckCommitMessage(msg); err != nil {
		return err
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	untracked := "--untracked-files=no"
	if withNew {
		untracked = "--untracked-files=all"
	}
	if gitOut(l.Root, "status", "--porcelain", untracked) == "" {
		return fmt.Errorf("нечего коммитить (новые файлы коммитятся с --new)")
	}
	if withNew {
		if out, err := exec.Command("git", "-C", l.Root, "add", "-A").CombinedOutput(); err != nil {
			return fmt.Errorf("git add: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if out, err := exec.Command("git", "-C", l.Root, "commit", "-q", "-a", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %v: %s", err, strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(env.Out, "✓ %s %s\n", gitOut(l.Root, "rev-parse", "--short=7", "HEAD"), msg)
	if un := gitOut(l.Root, "ls-files", "--others", "--exclude-standard"); un != "" {
		fmt.Fprintln(env.Out, "· новые файлы не включены в коммит (sky commit --new добавит их):")
		for _, f := range strings.Split(un, "\n") {
			fmt.Fprintf(env.Out, "  ? %s\n", f)
		}
	}
	return nil
}

// gitOut — вывод git в корне репозитория; при ошибке пустая строка.
func gitOut(root string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
