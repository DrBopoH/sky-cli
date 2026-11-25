package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/publish"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const publishUsage = `sky publish [--build build/ММДД-ЧЧММ] [--title "название"] [--dry]`

func publishCommand() Command {
	return Command{
		Name:      "publish",
		Usage:     publishUsage,
		Summary:   summaryPublish,
		NeedsInit: true,
		Run:       withLook(runPublish, false),
	}
}

func runPublish(a *App, env Env, args []string) error {
	var o publish.Options
	for i := 0; i < len(args); i++ {
		x := args[i]
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s требует значение: %s", x, publishUsage)
			}
			i++
			return args[i], nil
		}
		var err error
		switch x {
		case "--dry":
			o.Dry = true
		case "--build":
			o.Build, err = val()
		case "--title":
			o.Title, err = val()
		default:
			if strings.HasPrefix(x, "--") {
				return fmt.Errorf("неизвестный флаг «%s»: %s", x, publishUsage)
			}
			return fmt.Errorf("лишний аргумент «%s»: %s", x, publishUsage)
		}
		if err != nil {
			return err
		}
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	res, err := publish.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if err != nil {
		return err
	}
	if o.Dry {
		fmt.Fprintln(env.Out, "· --dry: ничего не изменено")
	}
	return nil
}
