package apply

import (
	"fmt"
	"os"

	"github.com/DrBopoH/sky-cli/internal/gitx"
)

// record — одна запись history/ на время её создания. Её ведут все три пути (патч, откат, ручной коммит):
// снимок «до», запись файлов, коммит, откат записи при ошибке и список строк результата устроены один раз.
type record struct {
	stage string
}

// newRecord создаёт папку записи и кладёт в неё снимок «до» для changes.
func newRecord(historyDir, name, source string, changes []change) (*record, error) {
	stage, err := nextStageDir(historyDir, name)
	if err != nil {
		return nil, err
	}
	if err := saveBefore(stage, source, changes); err != nil {
		os.RemoveAll(stage)
		return nil, err
	}
	return &record{stage: stage}, nil
}

// drop снимает запись и возвращает err.
func (r *record) drop(err error) error {
	os.RemoveAll(r.stage)
	return err
}

// rewind возвращает файлы из снимка «до» и снимает запись. Если вернуть не удалось, запись остаётся: из неё можно откатиться вручную.
func (r *record) rewind(root string, err error) (Result, error) {
	if rerr := Restore(root, r.stage); rerr != nil {
		return Result{Stage: r.stage}, fmt.Errorf("%w; откат не завершён, снимок в %s: %v", err, r.stage, rerr)
	}
	return Result{}, r.drop(err)
}

// commit делает коммит из paths и пишет его хэш в запись. Пути из .gitignore пропускаются, о них говорят notes.
// Пустой hash — коммитить было нечего. При ошибке индекс очищается от paths.
func (r *record) commit(root, msg string, paths []string) (hash string, notes []string, err error) {
	hash, skipped, err := commitPaths(root, msg, paths)
	for _, p := range skipped {
		notes = append(notes, p+": в .gitignore, в коммит не включён")
	}
	if err != nil {
		gitx.Unstage(root, paths)
		return "", notes, err
	}
	if hash != "" {
		if full, err := gitx.Run(root, "rev-parse", "HEAD"); err == nil {
			noteCommit(r.stage, full)
		}
	}
	return hash, notes, nil
}

// marks — строки результата: изменённое с пометками «+», «~», «-», затем неизменное с «=».
func marks(changes []change, same []string) []Line {
	var out []Line
	for _, c := range changes {
		mark := map[string]string{StCreated: "+", StModified: "~", StDeleted: "-"}[c.Status]
		out = append(out, Line{mark, c.Path})
	}
	for _, p := range same {
		out = append(out, Line{"=", p})
	}
	return out
}
