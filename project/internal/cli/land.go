package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/feature"
	"github.com/DrBopoH/sky-cli/internal/land"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/sync"
)

const landUsage = `sky land "резюме" [--module M [--chain a,b,c] | --seed] [--from R.B.C+T] [--dry] [--local]`

func landCommand() Command {
	return Command{
		Name:      "land",
		Usage:     landUsage,
		Summary:   summaryLand,
		NeedsInit: true,
		Run:       withLook(runLand, false),
	}
}

func runLand(a *App, env Env, args []string) error {
	o := land.Options{Now: time.Now()}
	var rest []string
	local := false
	for i := 0; i < len(args); i++ {
		x := args[i]
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s требует значение: %s", x, landUsage)
			}
			i++
			return args[i], nil
		}
		var err error
		switch x {
		case "--seed":
			o.Seed = true
		case "--dry":
			o.Dry = true
		case "--local":
			local = true
		case "--module":
			o.Module, err = val()
		case "--from":
			o.From, err = val()
		case "--chain":
			var v string
			if v, err = val(); err == nil {
				o.Chain = strings.Split(v, ",")
			}
		default:
			if strings.HasPrefix(x, "--") {
				return fmt.Errorf("неизвестный флаг «%s»: %s", x, landUsage)
			}
			rest = append(rest, x)
		}
		if err != nil {
			return err
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("резюме одним аргументом в кавычках: %s", landUsage)
	}
	o.Summary = rest[0]
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	if o.Module == "" && len(o.Chain) == 0 && !o.Seed {
		// цель запомнена sky feature: флаги не нужны
		if _, t, ok := feature.Current(l.Root); ok {
			switch t.Kind {
			case feature.Module:
				o.Module, o.Chain = t.Module, t.Chain
			case feature.Seed:
				o.Seed = true
			case feature.Fix:
				fa := []string{o.Summary}
				if o.Dry {
					fa = append(fa, "--dry")
				}
				if local {
					fa = append(fa, "--local")
				}
				return runFix(a, env, fa)
			}
		}
	}
	return landAndPush(env, l.Root, o, local)
}

// landAndPush выполняет land, печатает отчёт и отправляет изменившиеся общие ветки (если не --local).
func landAndPush(env Env, root string, o land.Options, local bool) error {
	before := sync.Tips(root)
	res, err := land.Run(o)
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
	fmt.Fprintf(env.Out, "✓ %s (HEAD: %s)\n", res.Message, res.Branch)
	pushShared(env, root, before, local)
	return nil
}
