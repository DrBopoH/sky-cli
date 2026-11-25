package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/modules"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const modulesUsage = `sky modules [модуль]`

func modulesCommand() Command {
	return Command{
		Name:      "modules",
		Usage:     modulesUsage,
		Summary:   summaryModules,
		NeedsInit: true,
		Run:       runModules,
	}
}

func runModules(a *App, env Env, args []string) error {
	for _, x := range args {
		if strings.HasPrefix(x, "--") {
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, modulesUsage)
		}
	}
	if len(args) > 1 {
		return fmt.Errorf("модуль один: %s", modulesUsage)
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	m := ""
	if len(args) == 1 {
		m = args[0]
	}
	res, err := modules.Inspect(l.Root, m)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	return err
}
