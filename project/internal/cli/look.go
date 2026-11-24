package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/look"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func lookCommand() Command {
	return Command{
		Name:      "look",
		Usage:     "sky look",
		Summary:   "обновить .sky/foryou/: факты о репозитории (состояние, ветки, история по 128 коммитов на ветку, дерево файлов и граф веток до 1024 строк, записи stages); те же файлы обновляются в конце init, apply, commit и rollback",
		NeedsInit: true,
		Run: func(a *App, env Env, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("sky look без аргументов")
			}
			l, err := repo.Find(env.Cwd)
			if err != nil {
				return err
			}
			names, err := refreshForyou(l)
			if err != nil {
				return err
			}
			fmt.Fprintf(env.Out, "✓ .sky/foryou/: %s\n", strings.Join(names, ", "))
			return nil
		},
	}
}

// refreshForyou пересобирает .sky/foryou/ и возвращает имена файлов.
func refreshForyou(l repo.Layout) ([]string, error) {
	c, err := conf.Load(l.Conf)
	if err != nil {
		return nil, err
	}
	inbox, err := c.InboxDir()
	if err != nil {
		return nil, err
	}
	var patches []string
	if ps, err := listPatches(inbox); err == nil {
		for _, p := range ps {
			patches = append(patches, filepath.Base(p))
		}
	}
	files := look.Build(look.Input{Root: l.Root, Stages: l.Stages, Conf: c, Inbox: c.Inbox, Patches: patches})
	gen := make([]repo.Generated, len(files))
	names := make([]string, len(files))
	for i, f := range files {
		gen[i] = repo.Generated{Name: f.Name, Content: []byte(f.Content)}
		names[i] = f.Name
	}
	if _, err := l.SyncForyou(gen); err != nil {
		return nil, err
	}
	return names, nil
}

// withLook обновляет foryou после команды: при успехе всегда, при ошибке — если always (apply мог применить часть патчей).
// Сбой обновления не ломает команду: печатается предупреждение.
func withLook(run func(*App, Env, []string) error, always bool) func(*App, Env, []string) error {
	return func(a *App, env Env, args []string) error {
		err := run(a, env, args)
		if err == nil || always {
			if l, ferr := repo.Find(env.Cwd); ferr == nil {
				if _, lerr := refreshForyou(l); lerr != nil {
					fmt.Fprintf(env.Out, "⚠ foryou не обновлён: %v\n", lerr)
				}
			}
		}
		return err
	}
}
