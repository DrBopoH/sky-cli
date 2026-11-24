// Package history — записи .sky/stages/history/: формат manifest.txt, чтение записей, дописывание пометок, лимит хранения.
// Что с записями делать (откатывать, применять) решает apply; здесь только сама история.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	PatchTag  = "# patch: "
	CommitTag = "# commit: "
	BackupTag = "# backup: "
	// Keep — сколько записей хранит history/; столько же патчей можно откатить за раз.
	Keep = 16
)

// NoteCommit дописывает в manifest.txt записи хэш коммита, созданного патчем: по нему откат снимет коммит.
func NoteCommit(stageDir, full string) {
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s%s\n", CommitTag, full)
}

// NoteCreated дописывает в manifest.txt файлы, которые коммит с флагом new подобрал в дереве: откат их удалит.
func NoteCreated(stageDir, status string, paths []string) {
	if len(paths) == 0 {
		return
	}
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, p := range paths {
		fmt.Fprintf(f, "%s\t%s\n", status, p)
	}
}

// NoteBackup дописывает в manifest.txt записи имя полного снимка «до».
func NoteBackup(stageDir, name string) {
	f, err := os.OpenFile(filepath.Join(stageDir, "manifest.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s%s\n", BackupTag, name)
}

// Prune оставляет Keep последних записей.
func Prune(historyDir string) {
	es, _ := Entries(historyDir)
	for len(es) > Keep {
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
		case strings.HasPrefix(line, PatchTag):
			e.Source = strings.TrimPrefix(line, PatchTag)
		case strings.HasPrefix(line, BackupTag):
			e.Backup = strings.TrimSpace(strings.TrimPrefix(line, BackupTag))
		case strings.HasPrefix(line, CommitTag):
			e.Commit = strings.TrimSpace(strings.TrimPrefix(line, CommitTag))
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

// Last возвращает n последних записей (новые первыми).
func Last(historyDir string, n int) ([]Entry, error) {
	list, err := Entries(historyDir)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("в .sky/stages/history/ нет записей, откатывать нечего")
	}
	if n < 1 || n > Keep {
		return nil, fmt.Errorf("глубина отката от 1 до %d", Keep)
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
