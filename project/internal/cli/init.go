package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/scaffold"
)

//go:embed text/init-help.txt
var initHelp string

func initCommand() Command {
	return Command{
		Name:    "init",
		Usage:   "sky init [пути…]",
		Summary: "завести проект: git-репозиторий (если его нет), первый коммит в main и ветку dev (в новом репозитории), папки scope (по умолчанию project/), README.md и .gitignore (если их нет), .sky/; напр. sky init backend/ frontend/",
		Help:    initHelp,
		Run:     runInit,
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
	l, created, err := repo.FindOrInit(env.Cwd)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(env.Out, "✓ git init (ветка main) в %s\n", l.Root)
	}
	if err := l.MkDirs(); err != nil {
		return err
	}

	c, err := writeConf(env, l, scope)
	if err != nil {
		return err
	}

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

	made, err := scaffoldFiles(env, l, c)
	if err != nil {
		return err
	}

	changes, err := a.Heal(l)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		switch ch.Status {
		case repo.Created:
			fmt.Fprintf(env.Out, "✓ %s создан (только чтение)\n", ch.Path)
		case repo.Replaced:
			fmt.Fprintf(env.Out, "↻ %s перезаписан (отличался от генерируемого)\n", ch.Path)
		case repo.Removed:
			fmt.Fprintf(env.Out, "✗ %s удалён (лишний)\n", ch.Path)
		default:
			fmt.Fprintf(env.Out, "= %s актуален\n", ch.Path)
		}
	}

	// Новый репозиторий: первый коммит в main и сразу ветка dev, на которой работа продолжается.
	if !l.HasCommits() {
		if err := l.FirstCommit(firstCommitMsg, made); err != nil {
			fmt.Fprintf(env.Out, "⚠ первый коммит не создан: %v\n", err)
		} else {
			fmt.Fprintln(env.Out, "✓ первый коммит в main, ветка dev создана и активна")
		}
	}
	return nil
}

// firstCommitMsg — сообщение первого коммита в main.
const firstCommitMsg = "chore: init. At the moment: project created by sky"

// scaffoldFiles: .gitignore и README.md создаются только если их нет; иначе в .gitignore дописывается .sky/.
func scaffoldFiles(env Env, l repo.Layout, c conf.Conf) ([]string, error) {
	var made []string
	ok, err := repo.WriteIfAbsent(filepath.Join(l.Root, ".gitignore"), []byte(scaffold.Gitignore()))
	if err != nil {
		return nil, err
	}
	if ok {
		made = append(made, ".gitignore")
		fmt.Fprintln(env.Out, "✓ .gitignore создан (шаблон)")
	} else if added, err := l.EnsureIgnored(".sky/"); err != nil {
		return nil, err
	} else if added {
		fmt.Fprintln(env.Out, "✓ .gitignore: добавлено .sky/")
	}
	ok, err = repo.WriteIfAbsent(filepath.Join(l.Root, "README.md"), []byte(scaffold.Readme(filepath.Base(l.Root), c.Scope)))
	if err != nil {
		return nil, err
	}
	if ok {
		made = append(made, "README.md")
		fmt.Fprintln(env.Out, "✓ README.md создан (шаблон)")
	}
	return made, nil
}

// writeConf: нет файла — создаёт; есть и пути не заданы — не трогает; есть и пути заданы — меняет только scope.
func writeConf(env Env, l repo.Layout, scope []string) (conf.Conf, error) {
	b, err := os.ReadFile(l.Conf)
	switch {
	case err == nil:
		c, perr := conf.Parse(string(b))
		if perr != nil {
			return c, perr
		}
		if scope == nil {
			fmt.Fprintln(env.Out, "= sky.conf не тронут")
			return c, nil
		}
		c.Scope = scope
		for p := range c.Cmd {
			if !c.InScope(p) {
				fmt.Fprintf(env.Out, "⚠ команды секции [%s] убраны: папки нет в новом scope\n", p)
			}
		}
		fmt.Fprintf(env.Out, "↻ sky.conf: scope = %v\n", scope)
		return c, os.WriteFile(l.Conf, []byte(conf.Render(c)), 0o644)
	case os.IsNotExist(err):
		c := conf.Default()
		if scope != nil {
			c.Scope = scope
		}
		fmt.Fprintf(env.Out, "✓ sky.conf создан: scope = %v\n", c.Scope)
		return c, os.WriteFile(l.Conf, []byte(conf.Render(c)), 0o644)
	default:
		return conf.Default(), err
	}
}
