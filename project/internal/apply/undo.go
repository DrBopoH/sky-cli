package apply

import (
	"archive/zip"
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/guard"
	"github.com/DrBopoH/sky-cli/internal/history"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/notation"
)

// RunEffects применяет эффекты как новую запись history/ (с «до» затронутых файлов), при ошибке откатывает.
func RunEffects(root, historyDir, dirName, source string, effects []Effect) (Result, error) {
	if err := CheckEffects(root, effects); err != nil {
		return Result{}, err
	}
	changes, same, err := classify(root, effects)
	if err != nil {
		return Result{}, err
	}
	rec, err := newRecord(historyDir, dirName, source, changes)
	if err != nil {
		return Result{}, err
	}
	if err := execute(root, changes); err != nil {
		return rec.rewind(root, err)
	}
	return Result{Stage: rec.stage, Lines: marks(changes, same)}, nil
}

func label(es []history.Entry) string { // es: новые первыми
	if len(es) == 1 {
		return fmt.Sprintf("%04d", es[0].Num)
	}
	return fmt.Sprintf("%04d_%04d", es[len(es)-1].Num, es[0].Num)
}

// Undo (soft) откатывает n последних записей history/ одной новой записью. Если среди них есть коммиты,
// они должны подряд лежать на вершине ветки: тогда снимаются (reset --mixed), иначе отказ без изменений.
func Undo(root, historyDir string, n int) (Result, []history.Entry, error) {
	es, err := history.Last(historyDir, n)
	if err != nil {
		return Result{}, nil, err
	}
	k, err := topCommits(root, es)
	if err != nil {
		return Result{}, es, err
	}
	merged := map[string]Effect{}
	for _, e := range es { // от новых к старым: побеждает самая старая запись
		eff, err := inverse(e)
		if err != nil {
			return Result{}, es, err
		}
		for _, x := range eff {
			merged[x.Path] = x
		}
	}
	paths := make([]string, 0, len(merged))
	for p := range merged {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	effects := make([]Effect, len(paths))
	for i, p := range paths {
		effects[i] = merged[p]
	}
	src := "откат " + es[0].Name
	if len(es) > 1 {
		src = fmt.Sprintf("откат %d записей: %s..%s", len(es), es[len(es)-1].Name, es[0].Name)
	}
	res, err := RunEffects(root, historyDir, "rollback-"+label(es), src, effects)
	if err != nil {
		return Result{}, es, err
	}
	if k > 0 {
		if _, err := gitx.Run(root, "reset", "-q", "--mixed", fmt.Sprintf("HEAD~%d", k)); err != nil {
			if rerr := Restore(root, res.Stage); rerr != nil {
				return Result{Stage: res.Stage}, es, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, res.Stage, rerr)
			}
			os.RemoveAll(res.Stage)
			return Result{}, es, err
		}
		res.Notes = append(res.Notes, fmt.Sprintf("коммитов снято: %d (git reset --mixed HEAD~%d)", k, k))
	}
	history.Prune(historyDir)
	return res, es, nil
}

// topCommits считает коммиты записей es (новые первыми), которые нужно снять; они должны лежать подряд на вершине ветки.
// Коммиты, которых уже нет в ветке, пропускаются.
func topCommits(root string, es []history.Entry) (int, error) {
	k := 0
	for _, e := range es {
		if e.Commit == "" || !gitx.OK(root, "merge-base", "--is-ancestor", e.Commit, "HEAD") {
			continue
		}
		top, err := gitx.Run(root, "rev-parse", fmt.Sprintf("HEAD~%d", k))
		if err != nil || top != e.Commit {
			return 0, fmt.Errorf("запись %s создала коммит %.7s, но он не лежит подряд на вершине ветки: откат вручную (git)", e.Name, e.Commit)
		}
		k++
	}
	if k > 0 {
		if _, err := gitx.Run(root, "rev-parse", "--verify", "-q", fmt.Sprintf("HEAD~%d", k)); err != nil {
			return 0, fmt.Errorf("снимаемые коммиты доходят до корня истории: откат с их снятием не поддерживается")
		}
	}
	return k, nil
}

// UndoKeep снимает коммиты n последних записей, оставляя файлы как есть (reset --mixed). Записи остаются,
// у них только пропадает отметка коммита, поэтому обычный откат по-прежнему вернёт их файлы «до».
func UndoKeep(root, historyDir string, n int) (int, []history.Entry, error) {
	es, err := history.Last(historyDir, n)
	if err != nil {
		return 0, nil, err
	}
	k, err := topCommits(root, es)
	if err != nil {
		return 0, es, err
	}
	if k == 0 {
		return 0, es, fmt.Errorf("в последних записях (%d) нет коммитов, которые можно снять", len(es))
	}
	if _, err := gitx.Run(root, "reset", "-q", "--mixed", fmt.Sprintf("HEAD~%d", k)); err != nil {
		return 0, es, err
	}
	for _, e := range es {
		if e.Commit == "" {
			continue
		}
		mp := filepath.Join(e.Dir, "manifest.txt")
		b, err := os.ReadFile(mp)
		if err != nil {
			continue
		}
		os.WriteFile(mp, []byte(strings.ReplaceAll(string(b), "\n"+history.CommitTag, "\n# был коммит: ")), 0o644)
	}
	return k, es, nil
}

// inverse строит эффекты, обратные записи: созданное удаляется, изменённое и удалённое возвращается из before.zip.
func inverse(e history.Entry) ([]Effect, error) {
	man, err := os.ReadFile(filepath.Join(e.Dir, "manifest.txt"))
	if err != nil {
		return nil, err
	}
	recs, err := notation.Parse(string(man), 2)
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
	var out []Effect
	for _, r := range recs {
		status, p := r[0], r[1]
		if err := guard.Path(p); err != nil {
			return nil, err
		}
		if status == StCreated {
			out = append(out, Effect{Kind: Delete, Path: p})
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
		out = append(out, Effect{Kind: Write, Path: p, Content: b, Exec: f.Mode()&0o111 != 0})
	}
	return out, nil
}
