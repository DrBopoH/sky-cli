package cli

import (
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/history"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/work"
)

func rollbackCommand() Command {
	return Command{
		Name:      "rollback",
		Usage:     "sky rollback [--force | --keep] [N] | sky rollback list",
		Summary:   summaryRollback,
		NeedsInit: true,
		Run:       withLook(runRollback, false),
	}
}

const rollbackUsage = "sky rollback [--force | --keep] [N] | sky rollback list"

func runRollback(a *App, env Env, args []string) error {
	force, keep, list, n := false, false, false, 1
	numSet := false
	for _, x := range args {
		switch {
		case x == "--force":
			force = true
		case x == "--keep":
			keep = true
		case x == "list":
			list = true
		default:
			v, err := strconv.Atoi(x)
			if err != nil || numSet {
				return fmt.Errorf("%s", rollbackUsage)
			}
			n, numSet = v, true
		}
	}
	if (list && (force || keep || numSet)) || (force && keep) {
		return fmt.Errorf("%s", rollbackUsage)
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	if list {
		return rollbackList(env, l)
	}
	if keep {
		k, es, err := apply.UndoKeep(l.Root, l.History, n)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Out, "── --keep: записей %d (%s)\n  · коммитов снято: %d (git reset --mixed HEAD~%d), файлы остались в дереве\n", len(es), names(es), k, k)
		return nil
	}
	if force {
		return rollbackForce(env, l, n)
	}
	res, es, err := apply.Undo(l.Root, l.History, n)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "── откат записей: %d (%s)\n", len(es), names(es))
	printResult(env, l, res)
	return nil
}

func names(es []history.Entry) string {
	var s []string
	for _, e := range es {
		s = append(s, e.Name)
	}
	return strings.Join(s, ", ")
}

func printResult(env Env, l repo.Layout, res apply.Result) {
	for _, ln := range res.Lines {
		fmt.Fprintf(env.Out, "  %s %s\n", ln.Mark, ln.Path)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(env.Out, "  · %s\n", n)
	}
	rel, _ := filepath.Rel(l.Root, res.Stage)
	fmt.Fprintf(env.Out, "● запись %s\n", filepath.ToSlash(rel))
}

func rollbackList(env Env, l repo.Layout) error {
	list, err := history.Entries(l.History)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(env.Out, "(записей нет)")
	}
	for _, e := range list {
		extra := ""
		if e.Commit != "" {
			extra += fmt.Sprintf("  коммит %.7s", e.Commit)
		}
		if e.Backup != "" {
			extra += "  снимок " + e.Backup
		}
		fmt.Fprintf(env.Out, "%04d  %s  файлов: %d%s\n", e.Num, e.Source, e.Files, extra)
	}
	return nil
}

// rollbackForce печатает откат по полному снимку; сам откат — work.RollbackForce.
func rollbackForce(env Env, l repo.Layout, n int) error {
	f, err := work.RollbackForce(l, n)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "── откат --force: %d записей, до снимка %s\n", len(f.Entries), f.Backup)
	printResult(env, l, f.Result)
	return nil
}
