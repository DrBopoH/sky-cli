// Package work — рабочие процессы ядра над проектом: применить патч с полным снимком «до» и вернуть проект к снимку.
// Консоль и операции зовут эти функции, сами алгоритмов не содержат.
package work

import (
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/guard"
	"github.com/DrBopoH/sky-cli/internal/history"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/pack"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

// CheckClean отказывает, если в отслеживаемых файлах есть незакоммиченные правки.
func CheckClean(root string) error {
	if dirty := gitx.Dirty(root); dirty != "" {
		return fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки; закоммить их или sky apply --force:\n%s", dirty)
	}
	return nil
}

// Applied — результат применения патча: запись history и путь к полному снимку «до».
type Applied struct {
	apply.Result
	Backup string
}

// ApplyPatch снимает полный снимок проекта «до», применяет патч и связывает запись history со снимком.
func ApplyPatch(l repo.Layout, patch string, plan apply.Planner) (Applied, error) {
	name := filepath.Base(patch)
	bk, err := pack.Make(pack.Options{Root: l.Root, Backups: l.Backups, Kind: pack.Before, Label: name})
	if err != nil {
		return Applied{}, fmt.Errorf("%s: не удалось сделать снимок проекта, патч не применён: %w", name, err)
	}
	p, err := plan(l.Root, patch)
	if err != nil {
		return Applied{}, fmt.Errorf("%s: %w", name, err)
	}
	res, err := apply.Run(l.Root, l.History, patch, p)
	if err != nil {
		return Applied{}, fmt.Errorf("%s: %w", name, err)
	}
	history.NoteBackup(res.Stage, filepath.Base(bk.Zip))
	return Applied{Result: res, Backup: bk.Zip}, nil
}

// restorable — путь, который откат по снимку трогает: настройки и сгенерированное sky остаются как есть.
func restorable(p string) bool {
	return p != ".sky/sky.conf" && !strings.HasPrefix(p, ".sky/foryou/") && !strings.HasPrefix(p, ".sky/readme/")
}

// Forced — результат отката по полному снимку.
type Forced struct {
	Result  apply.Result
	Entries []history.Entry // откатанные записи, новые первыми
	Backup  string          // имя снимка, до которого вернулись
}

// RollbackForce возвращает проект к полному снимку «до» N-го с конца патча. Коммиты не трогаются, о них сообщает Result.Notes.
func RollbackForce(l repo.Layout, n int) (Forced, error) {
	es, err := history.Last(l.History, n)
	if err != nil {
		return Forced{}, err
	}
	target := es[len(es)-1]
	if target.Backup == "" {
		return Forced{}, fmt.Errorf("у записи %s нет полного снимка (это откат): используй sky rollback %d без --force", target.Name, n)
	}
	zipPath := filepath.Join(l.Backups, target.Backup)
	if _, err := os.Stat(zipPath); err != nil {
		return Forced{}, fmt.Errorf("снимок %s удалён (в backups/ хранится 16 последних): используй sky rollback %d без --force", target.Backup, n)
	}
	snap, err := pack.ReadSnapshot(zipPath)
	if err != nil {
		return Forced{}, fmt.Errorf("снимок %s не читается: %v", target.Backup, err)
	}
	var effects []apply.Effect
	skipped := 0
	for p, f := range snap {
		if !restorable(p) {
			continue
		}
		if guard.Path(p) != nil {
			skipped++
			continue
		}
		effects = append(effects, apply.Effect{Kind: apply.Write, Path: p, Content: f.Data, Exec: f.Exec})
	}
	for _, p := range gitx.Present(l.Root) {
		if _, ok := snap[p]; ok || !restorable(p) || guard.Path(p) != nil {
			continue
		}
		fi, err := os.Lstat(filepath.Join(l.Root, filepath.FromSlash(p)))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > pack.MaxFile {
			continue
		}
		effects = append(effects, apply.Effect{Kind: apply.Delete, Path: p})
	}
	sort.Slice(effects, func(i, j int) bool { return effects[i].Path < effects[j].Path })
	src := fmt.Sprintf("откат --force до снимка %s", target.Backup)
	res, err := apply.RunEffects(l.Root, l.History, fmt.Sprintf("rollback-force-%04d", target.Num), src, effects)
	if err != nil {
		return Forced{}, err
	}
	history.Prune(l.History)
	var changed []apply.Line
	for _, ln := range res.Lines {
		if ln.Mark != "=" {
			changed = append(changed, ln)
		}
	}
	res.Lines = changed
	if skipped > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("пропущено файлов, которые патч не принимает: %d", skipped))
	}
	if cs := history.Commits(es); len(cs) > 0 {
		var short []string
		for _, c := range cs {
			short = append(short, c[:7])
		}
		res.Notes = append(res.Notes, "коммиты этих патчей не тронуты: "+strings.Join(short, ", "))
	}
	return Forced{Result: res, Entries: es, Backup: target.Backup}, nil
}
