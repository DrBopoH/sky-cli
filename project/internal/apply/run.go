package apply

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DrBopoH/sky-cli/internal/gitx"
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
	rec, err := newRecord(stagesDir, patchPath, patchPath, changes)
	if err != nil {
		return Result{}, err
	}
	res := Result{Stage: rec.stage, Notes: plan.Notes}
	if err := execute(root, changes); err != nil {
		return rec.rewind(root, err)
	}
	if plan.Commit != "" {
		paths := make([]string, len(changes))
		have := map[string]bool{}
		for i, c := range changes {
			paths[i] = c.Path
			have[c.Path] = true
		}
		if plan.CommitOpts.New {
			fresh, err := gitx.Untracked(root)
			if err != nil {
				gitx.Unstage(root, paths)
				return rec.rewind(root, err)
			}
			var extra []string
			for _, p := range fresh {
				if have[p] || GuardPath(p) != nil {
					continue
				}
				extra = append(extra, p)
				res.Lines = append(res.Lines, Line{"+", p})
			}
			noteCreated(rec.stage, extra)
			paths = append(paths, extra...)
		}
		hash, notes, err := rec.commit(root, plan.Commit, paths)
		res.Notes = append(res.Notes, notes...)
		if err != nil {
			return rec.rewind(root, err)
		}
		switch {
		case hash != "":
			res.Commit = hash + " " + plan.Commit
		case len(paths) == 0:
			res.Notes = append(res.Notes, "файлы не изменились, коммит не создан")
		default:
			res.Notes = append(res.Notes, "в коммит нечего включать, коммит не создан")
		}
	}
	if err := moveFile(patchPath, filepath.Join(rec.stage, filepath.Base(patchPath))); err != nil {
		return res, fmt.Errorf("файлы записаны, но патч не перенесён в stages: %w", err)
	}
	res.Lines = append(res.Lines, marks(changes, same)...)
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
