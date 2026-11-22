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
		Usage:     `sky commit "тип: текст"`,
		Summary:   "закоммитить ручные правки: все изменения отслеживаемых файлов одним коммитом; новые файлы в коммит не входят (sky их перечислит); сообщение как у операции commit",
		NeedsInit: true,
		Run:       runCommit,
	}
}

func runCommit(a *App, env Env, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf(`сообщение одним аргументом в кавычках: sky commit "тип: текст"`)
	}
	msg := args[0]
	if err := ops.CheckCommitMessage(msg); err != nil {
		return err
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	if gitOut(l.Root, "status", "--porcelain", "--untracked-files=no") == "" {
		return fmt.Errorf("в отслеживаемых файлах нет изменений, коммитить нечего")
	}
	if out, err := exec.Command("git", "-C", l.Root, "commit", "-q", "-a", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %v: %s", err, strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(env.Out, "✓ %s %s\n", gitOut(l.Root, "rev-parse", "--short=7", "HEAD"), msg)
	if un := gitOut(l.Root, "ls-files", "--others", "--exclude-standard"); un != "" {
		fmt.Fprintln(env.Out, "· новые файлы не включены в коммит:")
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
