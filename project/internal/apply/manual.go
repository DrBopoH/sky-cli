package apply

import (
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/gitx"
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/commit"
)

// RunManual — ручные правки как запись history/: тот же снимок «до» (версии из HEAD), тот же коммит.
// Коммитятся изменения отслеживаемых файлов, с opts.New ещё и новые (кроме игнорируемых).
// Пути, которые патч не принял бы (GuardPath), коммитятся, но в запись не попадают.
func RunManual(root, historyDir, msg string, opts commit.Options) (Result, error) {
	if _, err := gitx.Run(root, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return Result{}, fmt.Errorf("в репозитории нет коммитов")
	}
	tracked, err := gitx.Z(root, "diff", "--name-only", "-z", "--no-renames", "HEAD")
	if err != nil {
		return Result{}, err
	}
	paths := tracked
	if opts.New {
		fresh, err := gitx.Untracked(root)
		if err != nil {
			return Result{}, err
		}
		paths = append(paths, fresh...)
	}
	if len(paths) == 0 {
		return Result{}, fmt.Errorf("нечего коммитить (новые файлы коммитятся с --new)")
	}
	var changes []change
	var notes []string
	for _, p := range paths {
		if GuardPath(p) != nil {
			notes = append(notes, p+": в запись отката не попал (патч такой путь не принимает)")
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(p))
		old, exe, inHead := gitx.Blob(root, p)
		_, serr := os.Lstat(abs)
		switch {
		case !inHead:
			changes = append(changes, change{Status: StCreated, Path: p})
		case serr != nil:
			changes = append(changes, change{Status: StDeleted, Path: p, Old: old, OldExe: exe})
		default:
			changes = append(changes, change{Status: StModified, Path: p, Old: old, OldExe: exe})
		}
	}
	stage, err := nextStageDir(historyDir, "commit")
	if err != nil {
		return Result{}, err
	}
	fail := func(err error) (Result, error) {
		os.RemoveAll(stage)
		return Result{}, err
	}
	if err := saveBefore(stage, "commit", changes); err != nil {
		return fail(err)
	}
	hash, skipped, err := commitPaths(root, msg, paths)
	if err != nil {
		gitx.Unstage(root, paths)
		return fail(err)
	}
	if hash == "" {
		return fail(fmt.Errorf("в коммит нечего включать (все изменения под .gitignore)"))
	}
	if full, err := gitx.Run(root, "rev-parse", "HEAD"); err == nil {
		noteCommit(stage, full)
	}
	res := Result{Stage: stage, Commit: hash + " " + msg, Notes: notes}
	for _, p := range skipped {
		res.Notes = append(res.Notes, p+": в .gitignore, в коммит не включён")
	}
	for _, c := range changes {
		mark := map[string]string{StCreated: "+", StModified: "~", StDeleted: "-"}[c.Status]
		res.Lines = append(res.Lines, Line{mark, c.Path})
	}
	PruneHistory(historyDir)
	return res, nil
}
