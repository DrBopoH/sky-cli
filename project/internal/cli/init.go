package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

//go:embed text/init-help.txt
var initHelp string

func initCommand() Command {
	return Command{
		Name:    "init",
		Usage:   "sky init [пути…]",
		Summary: "завести проект один раз: git-репозиторий (если его нет), .sky/ с sky.conf, папки scope (по умолчанию project/), строка .sky/ в .gitignore, первый коммит в main и ветка dev (в репозитории без коммитов); повторный запуск отказывает; напр. sky init backend/ frontend/",
		Help:    initHelp,
		Run:     withLook(runInit, false),
	}
}

func runInit(a *App, env Env, args []string) error {
	var scope []string
	var err error
	if len(args) > 0 {
		if scope, err = conf.ParseScope(args); err != nil {
			return err
		}
	}
	// первый коммит нужен в репозитории без истории: без имени и почты отказываем до любых изменений
	if cur, ferr := repo.Find(env.Cwd); ferr != nil || !cur.HasCommits() {
		if !repo.CanCommit(env.Cwd) {
			return fmt.Errorf("git не знает имя и почту автора: задай git config user.name и user.email и повтори sky init")
		}
	}
	l, created, err := repo.FindOrInit(env.Cwd)
	if err != nil {
		return err
	}
	if _, err := os.Stat(l.Conf); err == nil {
		return fmt.Errorf("проект уже инициализирован (%s): init выполняется один раз; scope меняется правкой sky.conf", l.Conf)
	}
	if created {
		fmt.Fprintf(env.Out, "✓ git init (ветка main) в %s\n", l.Root)
	}
	if err := l.MkDirs(); err != nil {
		return err
	}
	c := conf.Default()
	if scope != nil {
		c.Scope = scope
	}
	if err := os.WriteFile(l.Conf, []byte(conf.Render(c)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "✓ sky.conf создан: scope = %v\n", c.Scope)

	for _, s := range c.Scope {
		if s == "./" {
			continue
		}
		dir := filepath.Join(l.Root, s)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			fmt.Fprintf(env.Out, "✓ %s создана\n", s)
		}
	}

	// sky держит свои файлы в .sky/ и опирается на то, что git их не видит.
	var made []string
	if added, err := l.EnsureIgnored(".sky/"); err != nil {
		return err
	} else if added {
		made = append(made, ".gitignore")
		fmt.Fprintln(env.Out, "✓ .gitignore: добавлено .sky/")
	}

	changes, err := a.Heal(l)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		if ch.Status == repo.Created {
			fmt.Fprintf(env.Out, "✓ %s создан (только чтение)\n", ch.Path)
		}
	}

	// Новый репозиторий: первый коммит в main и сразу ветка dev, на которой работа продолжается.
	if !l.HasCommits() {
		if err := l.FirstCommit(firstCommitMsg, made); err != nil {
			return fmt.Errorf("первый коммит не создан: %w", err)
		}
		fmt.Fprintln(env.Out, "✓ первый коммит в main, ветка dev создана и активна")
	}
	return nil
}

// firstCommitMsg — сообщение первого коммита в main.
const firstCommitMsg = "chore: init. At the moment: project created by sky"
