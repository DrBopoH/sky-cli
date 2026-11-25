package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/feature"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const featureUsage = `sky feature имя [--module M [--chain a,b,c] | --seed | --fix [--build build/ММДД-ЧЧММ]] [--dry]`

func featureCommand() Command {
	return Command{
		Name:      "feature",
		Usage:     featureUsage,
		Summary:   summaryFeature,
		NeedsInit: true,
		Run:       withLook(runFeature, false),
	}
}

func runFeature(a *App, env Env, args []string) error {
	o := feature.Options{Now: time.Now()}
	var rest []string
	for i := 0; i < len(args); i++ {
		x := args[i]
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s требует значение: %s", x, featureUsage)
			}
			i++
			return args[i], nil
		}
		var err error
		switch x {
		case "--seed":
			o.Seed = true
		case "--fix":
			o.Fix = true
		case "--dry":
			o.Dry = true
		case "--module":
			o.Module, err = val()
		case "--build":
			o.Build, err = val()
		case "--chain":
			var v string
			if v, err = val(); err == nil {
				o.Chain = strings.Split(v, ",")
			}
		default:
			if strings.HasPrefix(x, "--") {
				return fmt.Errorf("неизвестный флаг «%s»: %s", x, featureUsage)
			}
			rest = append(rest, x)
		}
		if err != nil {
			return err
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("имя фичи одним аргументом: %s", featureUsage)
	}
	o.Name = rest[0]
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	res, err := feature.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if err != nil {
		return err
	}
	if o.Dry {
		fmt.Fprintln(env.Out, "· --dry: ничего не изменено")
		return nil
	}
	fmt.Fprintf(env.Out, "✓ %s (от %s)\n", res.Branch, res.Base)
	return nil
}
