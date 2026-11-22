package apply

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/ops"
)

const commitTag = "# commit: "

// noteCommit дописывает в manifest.txt записи хэш коммита, созданного патчем: по нему откат снимет коммит.
func noteCommit(stageDir, full string) {
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s%s\n", commitTag, full)
}

// Entry — одна запись в .sky/stages/.
type Entry struct {
	Name   string // NNNN-время-имя
	Num    int
	Source string // имя патча или «откат …» из manifest.txt
	Files  int
	Commit string // полный хэш коммита патча; пусто — коммита не было
	Dir    string
}

func readStage(dir string) (Entry, error) {
	e := Entry{Dir: dir, Name: filepath.Base(dir)}
	if _, err := fmt.Sscanf(e.Name, "%04d-", &e.Num); err != nil {
		return e, fmt.Errorf("%s: нет номера записи", e.Name)
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.txt"))
	if err != nil {
		return e, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "# patch: "):
			e.Source = strings.TrimPrefix(line, "# patch: ")
		case strings.HasPrefix(line, commitTag):
			e.Commit = strings.TrimSpace(strings.TrimPrefix(line, commitTag))
		case line != "" && !strings.HasPrefix(line, "#"):
			e.Files++
		}
	}
	return e, nil
}

// Entries — записи stages/ от старых к новым.
func Entries(stagesDir string) ([]Entry, error) {
	ds, err := os.ReadDir(stagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, d := range ds {
		if !d.IsDir() {
			continue
		}
		e, err := readStage(filepath.Join(stagesDir, d.Name()))
		if err != nil {
			continue // посторонняя папка
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out, nil
}

// inverse строит эффекты, обратные записи: созданное удаляется, изменённое и удалённое возвращается из before.zip.
func inverse(e Entry) ([]ops.Effect, error) {
	man, err := os.ReadFile(filepath.Join(e.Dir, "manifest.txt"))
	if err != nil {
		return nil, err
	}
	recs, err := ops.Parse(string(man), 2)
	if err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(filepath.Join(e.Dir, "before.zip"))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	old := map[string]*zip.File{}
	for _, f := range zr.File {
		old[f.Name] = f
	}
	var out []ops.Effect
	for _, r := range recs {
		status, p := r[0], r[1]
		if err := GuardPath(p); err != nil {
			return nil, err
		}
		if status == StCreated {
			out = append(out, ops.Effect{Kind: ops.Delete, Path: p})
			continue
		}
		f, ok := old[p]
		if !ok {
			return nil, fmt.Errorf("%s: нет в снимке «до»", p)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, ops.Effect{Kind: ops.Write, Path: p, Content: b, Exec: f.Mode()&0o111 != 0})
	}
	return out, nil
}

// Undo откатывает последнюю запись stages/. Откат — тоже запись (с «до» текущего состояния), поэтому повторный
// откат возвращает всё назад. Если запись создала коммит и он вершина ветки, коммит снимается (reset --mixed HEAD~1).
func Undo(root, stagesDir string) (Result, Entry, error) {
	list, err := Entries(stagesDir)
	if err != nil {
		return Result{}, Entry{}, err
	}
	if len(list) == 0 {
		return Result{}, Entry{}, fmt.Errorf("в .sky/stages/ нет записей, откатывать нечего")
	}
	target := list[len(list)-1]

	var parent bool
	if target.Commit != "" {
		head, err := git(root, "rev-parse", "HEAD")
		if err != nil || head != target.Commit {
			return Result{}, target, fmt.Errorf("запись %s создала коммит %.7s, но он уже не вершина ветки: откат вручную (git)", target.Name, target.Commit)
		}
		if _, err := git(root, "rev-parse", "--verify", "-q", "HEAD~1"); err != nil {
			return Result{}, target, fmt.Errorf("коммит %.7s корневой: откат с его снятием не поддерживается", target.Commit)
		}
		parent = true
	}

	effects, err := inverse(target)
	if err != nil {
		return Result{}, target, err
	}
	if err := checkEffects(root, effects); err != nil {
		return Result{}, target, err
	}
	changes, same, err := classify(root, effects)
	if err != nil {
		return Result{}, target, err
	}
	stage, err := nextStageDir(stagesDir, fmt.Sprintf("rollback-%04d", target.Num))
	if err != nil {
		return Result{}, target, err
	}
	fail := func(err error) (Result, Entry, error) {
		os.RemoveAll(stage)
		return Result{}, target, err
	}
	if err := saveBefore(stage, fmt.Sprintf("откат %s", target.Name), changes); err != nil {
		return fail(err)
	}
	if err := execute(root, changes); err != nil {
		if rerr := Restore(root, stage); rerr != nil {
			return Result{Stage: stage}, target, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
		}
		return fail(err)
	}
	res := Result{Stage: stage}
	if parent {
		if _, err := git(root, "reset", "-q", "--mixed", "HEAD~1"); err != nil {
			if rerr := Restore(root, stage); rerr != nil {
				return Result{Stage: stage}, target, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
			}
			return fail(err)
		}
		res.Notes = append(res.Notes, fmt.Sprintf("коммит %.7s снят (git reset --mixed HEAD~1)", target.Commit))
	}
	for _, c := range changes {
		mark := map[string]string{StCreated: "+", StModified: "~", StDeleted: "-"}[c.Status]
		res.Lines = append(res.Lines, Line{mark, c.Path})
	}
	for _, p := range same {
		res.Lines = append(res.Lines, Line{"=", p})
	}
	return res, target, nil
}
