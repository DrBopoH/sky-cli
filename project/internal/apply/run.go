package apply

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Line struct {
	Mark string // "+" создан, "~" изменён, "-" удалён, "=" без изменений
	Path string
}

type Result struct {
	Stage  string // папка записи в .sky/stages/history/
	Commit string // «хэш сообщение» созданного коммита; пусто — коммита не было
	Lines  []Line
	Notes  []string
}

// Run применяет один патч. Проверка и план строятся до любой записи; ошибка при записи откатывает уже сделанное.
// Применённый патч переносится в запись history/; при ошибке остаётся там, где лежал.
func Run(root, stagesDir, patchPath string) (Result, error) {
	plan, err := Build(root, patchPath)
	if err != nil {
		return Result{}, err
	}
	changes, same, err := classify(root, plan.Effects)
	if err != nil {
		return Result{}, err
	}
	stage, err := nextStageDir(stagesDir, patchPath)
	if err != nil {
		return Result{}, err
	}
	res := Result{Stage: stage, Notes: plan.Notes}
	fail := func(err error) (Result, error) {
		os.RemoveAll(stage)
		return Result{}, err
	}
	if err := saveBefore(stage, patchPath, changes); err != nil {
		return fail(err)
	}
	if err := execute(root, changes); err != nil {
		if rerr := Restore(root, stage); rerr != nil {
			return Result{Stage: stage}, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
		}
		return fail(err)
	}
	if plan.Commit != "" {
		paths := make([]string, len(changes))
		have := map[string]bool{}
		for i, c := range changes {
			paths[i] = c.Path
			have[c.Path] = true
		}
		if plan.New {
			fresh, err := untracked(root)
			if err != nil {
				unstage(root, paths)
				if rerr := Restore(root, stage); rerr != nil {
					return Result{Stage: stage}, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
				}
				return fail(err)
			}
			var extra []string
			for _, p := range fresh {
				if have[p] || GuardPath(p) != nil {
					continue
				}
				extra = append(extra, p)
				res.Lines = append(res.Lines, Line{"+", p})
			}
			noteCreated(stage, extra)
			paths = append(paths, extra...)
		}
		hash, skipped, err := commitPaths(root, plan.Commit, paths)
		for _, p := range skipped {
			res.Notes = append(res.Notes, p+": в .gitignore, в коммит не включён")
		}
		if err != nil {
			unstage(root, paths)
			if rerr := Restore(root, stage); rerr != nil {
				return Result{Stage: stage}, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
			}
			return fail(err)
		}
		switch {
		case hash != "":
			res.Commit = hash + " " + plan.Commit
			if full, err := git(root, "rev-parse", "HEAD"); err == nil {
				noteCommit(stage, full)
			}
		case len(paths) == 0:
			res.Notes = append(res.Notes, "файлы не изменились, коммит не создан")
		default:
			res.Notes = append(res.Notes, "в коммит нечего включать, коммит не создан")
		}
	}
	if err := moveFile(patchPath, filepath.Join(stage, filepath.Base(patchPath))); err != nil {
		return res, fmt.Errorf("файлы записаны, но патч не перенесён в stages: %w", err)
	}
	for _, c := range changes {
		mark := map[string]string{StCreated: "+", StModified: "~", StDeleted: "-"}[c.Status]
		res.Lines = append(res.Lines, Line{mark, c.Path})
	}
	for _, p := range same {
		res.Lines = append(res.Lines, Line{"=", p})
	}
	PruneHistory(stagesDir)
	return res, nil
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return errors.Join(err, os.Remove(dst))
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
