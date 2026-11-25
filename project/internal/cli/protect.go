package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/protect"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const protectUsage = `sky protect [--dry]`

func protectCommand() Command {
	return Command{
		Name:      "protect",
		Usage:     protectUsage,
		Summary:   summaryProtect,
		NeedsInit: true,
		Run:       runProtect,
	}
}

func runProtect(a *App, env Env, args []string) error {
	var o protect.Options
	for _, x := range args {
		switch {
		case x == "--dry":
			o.Dry = true
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, protectUsage)
		default:
			return fmt.Errorf("лишний аргумент «%s»: %s", x, protectUsage)
		}
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	res, err := protect.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if err == nil && o.Dry {
		fmt.Fprintln(env.Out, "· --dry: на GitHub ничего не изменено")
	}
	return err
}
