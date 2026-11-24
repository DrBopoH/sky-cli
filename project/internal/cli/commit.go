package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/commit"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func commitCommand() Command {
	return Command{
		Name:      "commit",
		Usage:     `sky commit [--new] "тип: текст"`,
		Summary:   summaryCommit,
		NeedsInit: true,
		Run:       withLook(runCommit, false),
	}
}

func runCommit(a *App, env Env, args []string) error {
	var opts commit.Options
	var rest []string
	for _, x := range args {
		if fl, ok := strings.CutPrefix(x, "--"); ok {
			if err := opts.Set(fl); err != nil {
				return fmt.Errorf("%s: %w", x, err)
			}
			continue
		}
		rest = append(rest, x)
	}
	if len(rest) != 1 {
		return fmt.Errorf(`сообщение одним аргументом в кавычках: sky commit [--new] "тип: текст"`)
	}
	msg := rest[0]
	if err := commit.CheckMessage(msg); err != nil {
		return err
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	res, err := apply.RunManual(l.Root, l.History, msg, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "✓ %s\n", res.Commit)
	for _, ln := range res.Lines {
		fmt.Fprintf(env.Out, "  %s %s\n", ln.Mark, ln.Path)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(env.Out, "  · %s\n", n)
	}
	rel, _ := filepath.Rel(l.Root, res.Stage)
	fmt.Fprintf(env.Out, "● запись %s\n", filepath.ToSlash(rel))
	if len(res.Left) > 0 {
		fmt.Fprintln(env.Out, "· новые файлы не включены в коммит (sky commit --new добавит их):")
		for _, f := range res.Left {
			fmt.Fprintf(env.Out, "  ? %s\n", f)
		}
	}
	return nil
}
