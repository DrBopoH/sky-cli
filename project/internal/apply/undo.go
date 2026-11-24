package apply

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/ops"
)

const (
	commitTag = "# commit: "
	backupTag = "# backup: "
	// KeepHistory — сколько записей хранит history/; столько же патчей можно откатить за раз.
	KeepHistory = 16
)

// noteCommit дописывает в manifest.txt записи хэш коммита, созданного патчем: по нему откат снимет коммит.
func noteCommit(stageDir, full string) {
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s%s\n", commitTag, full)
}

// noteCreated дописывает в manifest.txt файлы, которые коммит с флагом new подобрал в дереве: откат их удалит.
func noteCreated(stageDir string, paths []string) {
	if len(paths) == 0 {
		return
	}
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, p := range paths {
		fmt.Fprintf(f, "%s\t%s\n", StCreated, p)
	}
}

// NoteBackup дописывает в manifest.txt записи имя полного снимка «до».
func NoteBackup(stageDir, name string) {
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s%s\n", backupTag, name)
}

// PruneHistory оставляет KeepHistory последних записей.
func PruneHistory(historyDir string) {
	es, _ := Entries(historyDir)
	for len(es) > KeepHistory {
		os.RemoveAll(es[0].Dir)
		es = es[1:]
	}
}

// Entry — одна запись в .sky/stages/history/.
type Entry struct {
	Name   string // NNNN-время-имя
	Num    int
	Source string // имя патча или «откат …» из manifest.txt
	Files  int
	Commit string // полный хэш коммита патча; пусто — коммита не было
	Backup string // имя полного снимка «до» в backups/; пусто — снимка нет (откат)
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
		case strings.HasPrefix(line, backupTag):
			e.Backup = strings.TrimSpace(strings.TrimPrefix(line, backupTag))
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

func gitOK(root string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", root}, args...)...).Run() == nil
}

// Commits возвращает хэши коммитов записей (пустые пропускаются).
func Commits(es []Entry) []string {
	var out []string
	for _, e := range es {
		if e.Commit != "" {
			out = append(out, e.Commit)
		}
	}
	return out
}

// RunEffects применяет эффекты как новую запись history/ (с «до» затронутых файлов), при ошибке откатывает.
func RunEffects(root, historyDir, dirName, source string, effects []ops.Effect) (Result, error) {
	if err := checkEffects(root, effects); err != nil {
		return Result{}, err
	}
	changes, same, err := classify(root, effects)
	if err != nil {
		return Result{}, err
	}
	stage, err := nextStageDir(historyDir, dirName)
	if err != nil {
		return Result{}, err
	}
	fail := func(err error) (Result, error) {
		os.RemoveAll(stage)
		return Result{}, err
	}
	if err := saveBefore(stage, source, changes); err != nil {
		return fail(err)
	}
	if err := execute(root, changes); err != nil {
		if rerr := Restore(root, stage); rerr != nil {
			return Result{Stage: stage}, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, stage, rerr)
		}
		return fail(err)
	}
	res := Result{Stage: stage}
	for _, c := range changes {
		mark := map[string]string{StCreated: "+", StModified: "~", StDeleted: "-"}[c.Status]
		res.Lines = append(res.Lines, Line{mark, c.Path})
	}
	for _, p := range same {
		res.Lines = append(res.Lines, Line{"=", p})
	}
	return res, nil
}

// Last возвращает n последних записей (новые первыми).
func Last(historyDir string, n int) ([]Entry, error) {
	list, err := Entries(historyDir)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("в .sky/stages/history/ нет записей, откатывать нечего")
	}
	if n < 1 || n > KeepHistory {
		return nil, fmt.Errorf("глубина отката от 1 до %d", KeepHistory)
	}
	if n > len(list) {
		return nil, fmt.Errorf("в history/ записей: %d, откатить %d нельзя", len(list), n)
	}
	out := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, list[len(list)-1-i])
	}
	return out, nil
}

func label(es []Entry) string { // es: новые первыми
	if len(es) == 1 {
		return fmt.Sprintf("%04d", es[0].Num)
	}
	return fmt.Sprintf("%04d_%04d", es[len(es)-1].Num, es[0].Num)
}

// Undo (soft) откатывает n последних записей history/ одной новой записью. Если среди них есть коммиты,
// они должны подряд лежать на вершине ветки: тогда снимаются (reset --mixed), иначе отказ без изменений.
func Undo(root, historyDir string, n int) (Result, []Entry, error) {
	es, err := Last(historyDir, n)
	if err != nil {
		return Result{}, nil, err
	}
	k, err := topCommits(root, es)
	if err != nil {
		return Result{}, es, err
	}
	merged := map[string]ops.Effect{}
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
	effects := make([]ops.Effect, len(paths))
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
		if _, err := git(root, "reset", "-q", "--mixed", fmt.Sprintf("HEAD~%d", k)); err != nil {
			if rerr := Restore(root, res.Stage); rerr != nil {
				return Result{Stage: res.Stage}, es, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, res.Stage, rerr)
			}
			os.RemoveAll(res.Stage)
			return Result{}, es, err
		}
		res.Notes = append(res.Notes, fmt.Sprintf("коммитов снято: %d (git reset --mixed HEAD~%d)", k, k))
	}
	PruneHistory(historyDir)
	return res, es, nil
}

// topCommits считает коммиты записей es (новые первыми), которые нужно снять; они должны лежать подряд на вершине ветки.
// Коммиты, которых уже нет в ветке, пропускаются.
func topCommits(root string, es []Entry) (int, error) {
	k := 0
	for _, e := range es {
		if e.Commit == "" || !gitOK(root, "merge-base", "--is-ancestor", e.Commit, "HEAD") {
			continue
		}
		top, err := git(root, "rev-parse", fmt.Sprintf("HEAD~%d", k))
		if err != nil || top != e.Commit {
			return 0, fmt.Errorf("запись %s создала коммит %.7s, но он не лежит подряд на вершине ветки: откат вручную (git)", e.Name, e.Commit)
		}
		k++
	}
	if k > 0 {
		if _, err := git(root, "rev-parse", "--verify", "-q", fmt.Sprintf("HEAD~%d", k)); err != nil {
			return 0, fmt.Errorf("снимаемые коммиты доходят до корня истории: откат с их снятием не поддерживается")
		}
	}
	return k, nil
}

// UndoKeep снимает коммиты n последних записей, оставляя файлы как есть (reset --mixed). Записи остаются,
// у них только пропадает отметка коммита, поэтому обычный откат по-прежнему вернёт их файлы «до».
func UndoKeep(root, historyDir string, n int) (int, []Entry, error) {
	es, err := Last(historyDir, n)
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
	if _, err := git(root, "reset", "-q", "--mixed", fmt.Sprintf("HEAD~%d", k)); err != nil {
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
		os.WriteFile(mp, []byte(strings.ReplaceAll(string(b), "\n"+commitTag, "\n# был коммит: ")), 0o644)
	}
	return k, es, nil
}
