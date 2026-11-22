package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func testCommand() Command {
	return Command{
		Name:      "test",
		Usage:     "sky test [папки…]",
		Summary:   "запустить команду test из sky.conf в папках scope (без аргументов — во всех, где она задана)",
		NeedsInit: true,
		Run:       scopedRunner("test"),
	}
}

func buildCommand() Command {
	return Command{
		Name:      "build",
		Usage:     "sky build [папки…]",
		Summary:   "запустить команду build из sky.conf в папках scope (без аргументов — во всех, где она задана)",
		NeedsInit: true,
		Run:       scopedRunner("build"),
	}
}

// scopedRunner запускает команду kind из секции [папка] sky.conf: из этой папки, через sh -c.
// Без аргументов — по всем папкам scope; ошибки собираются, выход ненулевой, если хоть одна папка не прошла.
func scopedRunner(kind string) func(a *App, env Env, args []string) error {
	return func(a *App, env Env, args []string) error {
		l, err := repo.Find(env.Cwd)
		if err != nil {
			return err
		}
		c, err := conf.Load(l.Conf)
		if err != nil {
			return err
		}
		targets := c.Scope
		if len(args) > 0 {
			if targets, err = conf.ParseScope(args); err != nil {
				return err
			}
			for _, t := range targets {
				if !c.InScope(t) {
					return fmt.Errorf("%s нет в scope: %s", t, strings.Join(c.Scope, " "))
				}
			}
		}
		var ran int
		var failed []string
		for _, t := range targets {
			cmd := c.Cmd[t].Get(kind)
			if cmd == "" {
				fmt.Fprintf(env.Out, "· %s: команда %s не задана\n", t, kind)
				continue
			}
			dir := filepath.Join(l.Root, t)
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				fmt.Fprintf(env.Err, "✗ %s: папки нет\n", t)
				failed = append(failed, t)
				continue
			}
			ran++
			fmt.Fprintf(env.Out, "── %s · %s\n", t, kind)
			x := exec.Command("sh", "-c", cmd)
			x.Dir, x.Stdout, x.Stderr = dir, env.Out, env.Err
			if err := x.Run(); err != nil {
				fmt.Fprintf(env.Err, "✗ %s · %s: %v\n", t, kind, err)
				failed = append(failed, t)
			}
		}
		switch {
		case len(failed) > 0:
			return fmt.Errorf("%s не прошёл: %s", kind, strings.Join(failed, " "))
		case ran == 0:
			return fmt.Errorf("команда %s не задана ни в одной папке; добавь секцию [папка/] с %s = … в .sky/sky.conf", kind, kind)
		}
		return nil
	}
}
