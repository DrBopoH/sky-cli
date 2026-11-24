package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/work"
)

func applyCommand() Command {
	return Command{
		Name:      "apply",
		Usage:     "sky apply [--force]",
		Summary:   "применить все patch*.zip из inbox от старых к новым: проверить, снять полный снимок проекта в .sky/stages/backups/ и «до» изменённых файлов в .sky/stages/history/, выполнить операции, записать файлы, перенести патч в history",
		NeedsInit: true,
		Run:       withLook(runApply, true),
	}
}

func runApply(a *App, env Env, args []string) error {
	force := false
	for _, x := range args {
		if x != "--force" {
			return fmt.Errorf("неизвестный аргумент «%s»; есть только --force", x)
		}
		force = true
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
	if !force {
		if err := work.CheckClean(l.Root); err != nil {
			return err
		}
	}
	for _, p := range patches {
		name := filepath.Base(p)
		fmt.Fprintf(env.Out, "── %s\n", name)
		res, err := work.ApplyPatch(l, p)
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
