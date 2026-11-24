package cli

import (
	"fmt"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func rollbackCommand() Command {
	return Command{
		Name:      "rollback",
		Usage:     "sky rollback [list]",
		Summary:   "откатить последнюю запись .sky/stages/ (применённый патч или откат): вернуть файлы «до», снять созданный патчем коммит, если он ещё вершина ветки; сам откат тоже запись, повторный sky rollback возвращает всё назад; list — показать записи",
		NeedsInit: true,
		Run:       withLook(runRollback, false),
	}
}

func runRollback(a *App, env Env, args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "list") {
		return fmt.Errorf("sky rollback [list]")
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		list, err := apply.Entries(l.Stages)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintln(env.Out, "(записей нет)")
		}
		for _, e := range list {
			commit := ""
			if e.Commit != "" {
				commit = fmt.Sprintf("  коммит %.7s", e.Commit)
			}
			fmt.Fprintf(env.Out, "%04d  %s  файлов: %d%s\n", e.Num, e.Source, e.Files, commit)
		}
		return nil
	}
	res, target, err := apply.Undo(l.Root, l.Stages)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "── откат записи %s\n", target.Name)
	for _, ln := range res.Lines {
		fmt.Fprintf(env.Out, "  %s %s\n", ln.Mark, ln.Path)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(env.Out, "  · %s\n", n)
	}
	rel, _ := filepath.Rel(l.Root, res.Stage)
	fmt.Fprintf(env.Out, "● запись %s\n", filepath.ToSlash(rel))
	return nil
}
