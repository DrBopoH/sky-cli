package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/sync"
)

const syncUsage = `sky sync [--dry]`

func syncCommand() Command {
	return Command{
		Name:      "sync",
		Usage:     syncUsage,
		Summary:   summarySync,
		NeedsInit: true,
		Run:       withLook(runSync, false),
	}
}

func runSync(a *App, env Env, args []string) error {
	var o sync.Options
	for _, x := range args {
		switch {
		case x == "--dry":
			o.Dry = true
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, syncUsage)
		default:
			return fmt.Errorf("лишний аргумент «%s»: %s", x, syncUsage)
		}
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	res, err := sync.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	return err
}
