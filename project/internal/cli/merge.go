package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/merge"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const mergeUsage = `sky merge <PR> [--dry] | sky merge --up "резюме" [--dry]`

func mergeCommand() Command {
	return Command{
		Name:      "merge",
		Usage:     mergeUsage,
		Summary:   summaryMerge,
		NeedsInit: true,
		Run:       withLook(runMerge, false),
	}
}

func runMerge(a *App, env Env, args []string) error {
	var o merge.Options
	var rest []string
	for _, x := range args {
		switch {
		case x == "--dry":
			o.Dry = true
		case x == "--up":
			o.Up = true
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, mergeUsage)
		default:
			rest = append(rest, x)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("нужен один аргумент (номер PR или резюме для --up): %s", mergeUsage)
	}
	if o.Up {
		o.Summary = rest[0]
	} else {
		o.PR = rest[0]
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	res, err := merge.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if err != nil {
		return err
	}
	if o.Dry {
		fmt.Fprintln(env.Out, "· --dry: ничего не изменено, не запушено и не закрыто")
	}
	return nil
}
