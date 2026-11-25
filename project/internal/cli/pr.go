package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/feature"
	"github.com/DrBopoH/sky-cli/internal/pr"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const prUsage = `sky pr "резюме" [--base ветка] [--draft] [--dry]`

func prCommand() Command {
	return Command{
		Name:      "pr",
		Usage:     prUsage,
		Summary:   summaryPr,
		NeedsInit: true,
		Run:       withLook(runPr, false),
	}
}

func runPr(a *App, env Env, args []string) error {
	var o pr.Options
	var rest []string
	for i := 0; i < len(args); i++ {
		switch x := args[i]; {
		case x == "--dry":
			o.Dry = true
		case x == "--draft":
			o.Draft = true
		case x == "--base":
			if i+1 >= len(args) {
				return fmt.Errorf("--base требует значение: %s", prUsage)
			}
			i++
			o.Base = args[i]
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, prUsage)
		default:
			rest = append(rest, x)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("резюме одним аргументом в кавычках: %s", prUsage)
	}
	o.Summary = rest[0]
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	o.Root = l.Root
	br, t, ok := feature.Current(l.Root)
	if !ok || !strings.HasPrefix(br, feature.Prefix) {
		return fmt.Errorf("sky pr работает на ветке фичи %sММДД-ЧЧММ/имя, заведённой sky feature (сейчас: %s)", feature.Prefix, orDetached(br))
	}
	o.Branch, o.Kind, o.Module, o.Chain = br, string(t.Kind), t.Module, t.Chain
	if t.Kind == feature.Fix && o.Base == "" {
		o.Base = t.Base // исправление идёт PR в ту сборку, от которой отведено
	}
	res, err := pr.Run(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if err != nil {
		return err
	}
	if o.Dry {
		fmt.Fprintln(env.Out, "· --dry: ничего не запушено и не открыто")
	}
	return nil
}
