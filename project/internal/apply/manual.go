package apply

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitBlob возвращает содержимое файла в HEAD и признак исполняемого; ok=false, если файла в HEAD нет.
func gitBlob(root, p string) (data []byte, exe, ok bool) {
	tree, err := exec.Command("git", "-C", root, "ls-tree", "HEAD", "--", p).Output()
	if err != nil || len(tree) == 0 {
		return nil, false, false
	}
	exe = strings.HasPrefix(string(tree), "100755")
	data, err = exec.Command("git", "-C", root, "cat-file", "blob", "HEAD:"+p).Output()
	return data, exe, err == nil
}

func gitZ(root string, args ...string) ([]string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v", args[0], err)
	}
	var r []string
	for _, s := range strings.Split(string(out), "\x00") {
		if s != "" {
			r = append(r, s)
		}
	}
	return r, nil
}

// RunManual — ручные правки как запись history/: тот же снимок «до» (версии из HEAD), тот же коммит.
// Коммитятся изменения отслеживаемых файлов, с withNew ещё и новые (кроме игнорируемых).
// Пути, которые патч не принял бы (GuardPath), коммитятся, но в запись не попадают.
func RunManual(root, historyDir, msg string, withNew bool) (Result, error) {
	if _, err := git(root, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return Result{}, fmt.Errorf("в репозитории нет коммитов")
	}
	tracked, err := gitZ(root, "diff", "--name-only", "-z", "--no-renames", "HEAD")
	if err != nil {
		return Result{}, err
	}
	paths := tracked
	if withNew {
		fresh, err := gitZ(root, "ls-files", "-z", "--others", "--exclude-standard")
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
		old, exe, inHead := gitBlob(root, p)
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
		unstage(root, paths)
		return fail(err)
	}
	if hash == "" {
		return fail(fmt.Errorf("в коммит нечего включать (все изменения под .gitignore)"))
	}
	if full, err := git(root, "rev-parse", "HEAD"); err == nil {
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
