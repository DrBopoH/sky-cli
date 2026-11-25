package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/feature"
	"github.com/DrBopoH/sky-cli/internal/fix"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const fixUsage = `sky fix "резюме" [--build build/ММДД-ЧЧММ] [--dry]`

func fixCommand() Command {
	return Command{
		Name:      "fix",
		Usage:     fixUsage,
		Summary:   summaryFix,
		NeedsInit: true,
		Run:       withLook(runFix, false),
	}
}

func runFix(a *App, env Env, args []string) error {
	var o fix.Options
	var rest []string
	for i := 0; i < len(args); i++ {
		switch x := args[i]; {
		case x == "--dry":
			o.Dry = true
		case x == "--build":
			if i+1 >= len(args) {
				return fmt.Errorf("--build требует значение: %s", fixUsage)
			}
			i++
			o.Build = args[i]
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, fixUsage)
		default:
			rest = append(rest, x)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("резюме одним аргументом в кавычках: %s", fixUsage)
	}
	o.Summary = rest[0]
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	if o.Build == "" {
		// ветка заведена sky feature --fix: исправляем ту сборку, от которой она отведена
		if _, t, ok := feature.Current(l.Root); ok && t.Kind == feature.Fix {
			o.Build = t.Base
		}
	}
	res, err := fix.Run(o)
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
	return nil
}
