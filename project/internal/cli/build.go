package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/build"
	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

const buildUsage = `sky build [a|b|s|m] ["название"] [--dry] [--from R.B.C+T]`

func buildCommand() Command {
	return Command{
		Name:      "build",
		Usage:     buildUsage,
		Summary:   summaryBuild,
		NeedsInit: true,
		Run:       withLook(runBuild, true),
	}
}

func runBuild(a *App, env Env, args []string) error {
	o := build.Options{Now: time.Now(), Out: env.Out}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch x := args[i]; {
		case x == "--dry":
			o.Dry = true
		case x == "--from":
			if i+1 >= len(args) {
				return fmt.Errorf("--from требует значение: %s", buildUsage)
			}
			i++
			o.From = args[i]
		case strings.HasPrefix(x, "--"):
			return fmt.Errorf("неизвестный флаг «%s»: %s", x, buildUsage)
		default:
			rest = append(rest, x)
		}
	}
	if len(rest) > 0 && len(rest[0]) == 1 && strings.Contains("absm", rest[0]) {
		o.State = rest[0][0]
		rest = rest[1:]
	}
	if len(rest) > 1 {
		return fmt.Errorf("название сборки одним аргументом в кавычках: %s", buildUsage)
	}
	if len(rest) == 1 {
		o.Title = rest[0]
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	c, err := conf.Load(l.Conf)
	if err != nil {
		return err
	}
	o.Root, o.Points = l.Root, c.Build
	res, err := build.Exec(o)
	for _, ln := range res.Lines {
		fmt.Fprintln(env.Out, ln)
	}
	if !o.Dry && (res.Done || len(res.Runs) > 0) {
		if werr := l.WriteForyouFile("build.md", []byte(res.Markdown)); werr != nil {
			fmt.Fprintf(env.Out, "⚠ build.md не записан: %v\n", werr)
		}
	}
	if err != nil {
		return err
	}
	if o.Dry {
		fmt.Fprintln(env.Out, "· --dry: ничего не запускалось и не менялось")
		return nil
	}
	fmt.Fprintf(env.Out, "✓ %s (ветка %s, HEAD: dev)\n", res.Message, res.Branch)
	return nil
}
