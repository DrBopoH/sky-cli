package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func initCommand() Command {
	return Command{
		Name:    "init",
		Usage:   "sky init [пути…]",
		Summary: "завести .sky/ (sky.conf, stages/, readme/, foryou/, clouds/); пути — scope, по умолчанию project/; напр. sky init backend/ frontend/",
		Run:     runInit,
	}
}

func runInit(a *App, env Env, args []string) error {
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	var scope []string
	if len(args) > 0 {
		if scope, err = conf.ParseScope(args); err != nil {
			return err
		}
	}
	if err := l.MkDirs(); err != nil {
		return err
	}

	c, err := writeConf(env, l, scope)
	if err != nil {
		return err
	}

	added, err := l.EnsureIgnored(".sky/")
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintln(env.Out, "✓ .gitignore: добавлено .sky/")
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

	for _, s := range c.Scope {
		if st, err := os.Stat(filepath.Join(l.Root, s)); err != nil || !st.IsDir() {
			fmt.Fprintf(env.Out, "⚠ папки %s пока нет — это только предупреждение\n", s)
		}
	}
	return nil
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
