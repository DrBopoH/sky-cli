package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/feature"
	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/land"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/work"
)

const applyUsage = `sky apply [--force] | sky apply --land "резюме" (--module M [--chain a,b,c] | --seed) [--local]`

func applyCommand() Command {
	return Command{
		Name:      "apply",
		Usage:     applyUsage,
		Summary:   summaryApply,
		NeedsInit: true,
		Run:       withLook(runApply, true),
	}
}

func runApply(a *App, env Env, args []string) error {
	force, seed, local := false, false, false
	var summary, module, chain string
	for i := 0; i < len(args); i++ {
		x := args[i]
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s требует значение: %s", x, applyUsage)
			}
			i++
			return args[i], nil
		}
		var err error
		switch x {
		case "--force":
			force = true
		case "--seed":
			seed = true
		case "--local":
			local = true
		case "--land":
			summary, err = val()
		case "--module":
			module, err = val()
		case "--chain":
			chain, err = val()
		default:
			return fmt.Errorf("неизвестный аргумент «%s»: %s", x, applyUsage)
		}
		if err != nil {
			return err
		}
	}
	landing := summary != ""
	if !landing && (seed || module != "" || chain != "" || local) {
		return fmt.Errorf("--module, --chain, --seed и --local работают только с --land: %s", applyUsage)
	}
	if landing && force {
		return fmt.Errorf("--land и --force несовместимы: слияние в граф идёт только с чистого дерева: %s", applyUsage)
	}
	if landing && (seed == (module != "") || (seed && chain != "")) {
		return fmt.Errorf("с --land нужна ровно одна цель: --module M [--chain a,b,c] или --seed: %s", applyUsage)
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	c, err := conf.Load(l.Conf)
	if err != nil {
		return err
	}
	inbox, err := c.InboxDir()
	if err != nil {
		return err
	}
	patches, err := listPatches(inbox)
	if err != nil {
		return err
	}
	if len(patches) == 0 {
		return fmt.Errorf("в %s нет patch*.zip", inbox)
	}
	if landing {
		return applyAndLand(a, env, l, patches[0], summary, module, chain, seed, local)
	}
	if !force {
		if err := work.CheckClean(l.Root); err != nil {
			return err
		}
	}
	for _, p := range patches {
		if err := applyOne(a, env, l, p); err != nil {
			return err
		}
	}
	return nil
}

// applyOne применяет один патч и печатает отчёт.
func applyOne(a *App, env Env, l repo.Layout, p string) error {
	fmt.Fprintf(env.Out, "── %s\n", filepath.Base(p))
	res, err := work.ApplyPatch(l, p, a.Plan)
	if err != nil {
		return err
	}
	for _, ln := range res.Lines {
		fmt.Fprintf(env.Out, "  %s %s\n", ln.Mark, ln.Path)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(env.Out, "  · %s\n", n)
	}
	if res.Commit != "" {
		fmt.Fprintf(env.Out, "● коммит %s\n", res.Commit)
	}
	rel, _ := filepath.Rel(l.Root, res.Stage)
	fmt.Fprintf(env.Out, "● запись %s\n", filepath.ToSlash(rel))
	brel, _ := filepath.Rel(l.Root, res.Backup)
	fmt.Fprintf(env.Out, "● снимок проекта до патча %s\n", filepath.ToSlash(brel))
	return nil
}

var slugBad = regexp.MustCompile(`[^a-z0-9]+`)

// featureName — имя ветки разбора из имени патча: patch-sky-3-publish.zip → sky-3-publish.
func featureName(patch string) string {
	n := strings.ToLower(strings.TrimSuffix(filepath.Base(patch), ".zip"))
	n = strings.TrimPrefix(n, "patch")
	n = strings.Trim(slugBad.ReplaceAllString(n, "-"), "-")
	if n == "" {
		n = "patch"
	}
	return n
}

// applyAndLand — голова над apply и land: патч уже готовая фича без шума, поэтому ветка wip ему не нужна. Самый старый
// патч из inbox применяется одним коммитом на detached HEAD от dev (запись и хэш коммита — в .sky/stages/history),
// land вливает этот коммит в слой по цели, HEAD остаётся на ветке слоя. Остальные патчи ждут следующего запуска.
// Если слияние не удалось, на коммите патча заводится wip для разбора.
func applyAndLand(a *App, env Env, l repo.Layout, patch, summary, module, chain string, seed bool, local bool) error {
	if err := work.CheckClean(l.Root); err != nil {
		return err
	}
	if _, err := gitx.Run(l.Root, "checkout", "-q", "--detach", "dev"); err != nil {
		return fmt.Errorf("нет ветки dev: %w", err)
	}
	if err := applyOne(a, env, l, patch); err != nil {
		return err
	}
	src := strings.TrimSpace(gitx.Try(l.Root, "rev-parse", "HEAD"))
	o := land.Options{Root: l.Root, Summary: summary, Module: module, Seed: seed, Source: src, Now: time.Now()}
	if chain != "" {
		o.Chain = strings.Split(chain, ",")
	}
	if err := landAndPush(env, l.Root, o, local); err != nil {
		wip := feature.Prefix + time.Now().Format(feature.DayStamp) + "/" + featureName(patch)
		if _, e2 := gitx.Run(l.Root, "checkout", "-q", "-b", wip); e2 != nil {
			return fmt.Errorf("%w\n  патч применён коммитом %s, ветку разбора завести не удалось: %v", err, src[:7], e2)
		}
		return fmt.Errorf("%w\n  патч лежит в ветке %s: исправь и повтори sky land с целью (--module/--chain или --seed)", err, wip)
	}
	if n, _ := listPatches(filepath.Dir(patch)); len(n) > 0 {
		fmt.Fprintf(env.Out, "· в inbox ещё патчей: %d (следующий — тем же sky apply --land …)\n", len(n))
	}
	return nil
}

// listPatches — patch*.zip из inbox от старых к новым (по времени изменения, затем по имени).
func listPatches(inbox string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(inbox, "patch*.zip"))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(inbox); err != nil {
		return nil, fmt.Errorf("inbox не найден: %s", inbox)
	}
	type item struct {
		path string
		mod  int64
	}
	items := make([]item, 0, len(paths))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		items = append(items, item{p, fi.ModTime().UnixNano()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].mod != items[j].mod {
			return items[i].mod < items[j].mod
		}
		return items[i].path < items[j].path
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out, nil
}
