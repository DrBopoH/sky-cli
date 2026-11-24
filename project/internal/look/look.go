// Package look собирает снимок состояния репозитория для .sky/foryou/: только факты, без оценок.
package look

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/conf"
)

const (
	MaxTreeLines     = 1024
	MaxBranchCommits = 128
)

// File — сгенерированный файл foryou.
type File struct {
	Name    string
	Content string
}

// Input — то, что снимок берёт не из git.
type Input struct {
	Root    string
	Stages  string
	Conf    conf.Conf
	Inbox   string   // раскрытая папка inbox
	Patches []string // имена patch*.zip в inbox
}

func git(root string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

var userinfo = regexp.MustCompile(`(://)[^/@\s]+@`)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Build собирает все файлы foryou.
func Build(in Input) []File {
	head := git(in.Root, "rev-parse", "--short=7", "HEAD")
	snap := "Снимок при HEAD " + head + "."
	if head == "" {
		snap = "Снимок: коммитов ещё нет."
	}
	files := []File{
		{"status.md", status(in, head, snap)},
		{"branches.md", branches(in, snap)},
		{"history.md", history(in, snap)},
		{"tree.md", tree(in, snap)},
		{"graph.md", graph(in, snap)},
		{"stages.md", stages(in, snap)},
	}
	return files
}

func orNone(ss []string, f func(string) string) string {
	if len(ss) == 0 {
		return "  нет\n"
	}
	var b strings.Builder
	for _, s := range ss {
		b.WriteString("  " + f(s) + "\n")
	}
	return b.String()
}

func status(in Input, head, snap string) string {
	var b strings.Builder
	b.WriteString("Состояние рабочего дерева: ветка, HEAD, незакоммиченное, remote, патчи в inbox, последняя запись history. Читай первым.\n" + snap + "\n\n")
	branch := git(in.Root, "branch", "--show-current")
	if branch == "" {
		branch = "(нет текущей ветки)"
	}
	fmt.Fprintf(&b, "ветка: %s\n", branch)
	if head != "" {
		fmt.Fprintf(&b, "HEAD: %s %s\n", head, git(in.Root, "log", "-1", "--format=%s"))
	} else {
		b.WriteString("HEAD: коммитов нет\n")
	}
	var remotes []string
	for _, r := range lines(git(in.Root, "remote", "-v")) {
		if strings.HasSuffix(r, "(fetch)") {
			remotes = append(remotes, userinfo.ReplaceAllString(strings.TrimSuffix(r, " (fetch)"), "$1"))
		}
	}
	if len(remotes) == 0 {
		b.WriteString("remote: нет\n")
	} else {
		b.WriteString("remote:\n" + orNone(remotes, func(s string) string { return strings.Replace(s, "\t", " ", 1) }))
	}
	fmt.Fprintf(&b, "scope: %s\n\n", strings.Join(in.Conf.Scope, ", "))

	var tracked []string
	for _, l := range lines(git(in.Root, "status", "--porcelain", "--untracked-files=no")) {
		tracked = append(tracked, l)
	}
	b.WriteString("изменено (отслеживаемые файлы):\n" + orNone(tracked, func(s string) string { return s }))
	var fresh []string
	for _, f := range lines(git(in.Root, "ls-files", "--others", "--exclude-standard")) {
		fresh = append(fresh, f)
	}
	b.WriteString("новые (не в git, не игнорируются):\n" + orNone(fresh, func(s string) string { return "? " + s }))
	b.WriteString("\n")
	inbox := in.Inbox
	if len(in.Patches) == 0 {
		fmt.Fprintf(&b, "inbox (%s): патчей нет\n", inbox)
	} else {
		fmt.Fprintf(&b, "inbox (%s): %s\n", inbox, strings.Join(in.Patches, ", "))
	}
	if es, _ := apply.Entries(in.Stages); len(es) > 0 {
		e := es[len(es)-1]
		commit := ""
		if e.Commit != "" {
			commit = fmt.Sprintf(", коммит %.7s", e.Commit)
		}
		fmt.Fprintf(&b, "последняя запись history: %04d %s%s\n", e.Num, e.Source, commit)
	} else {
		b.WriteString("последняя запись history: нет\n")
	}
	return b.String()
}

func branches(in Input, snap string) string {
	var b strings.Builder
	b.WriteString("Ветки репозитория: локальные и удалённые, вершина и тема последнего коммита. Читай, когда нужно знать, какие ветки есть.\n" + snap + "\n\n")
	cur := git(in.Root, "branch", "--show-current")
	section := func(title, ref string) {
		rows := lines(git(in.Root, "for-each-ref", "--format=%(refname:short)\t%(objectname:short=7)\t%(contents:subject)", ref))
		b.WriteString(title + "\n")
		if len(rows) == 0 {
			b.WriteString("  нет\n")
			return
		}
		for _, r := range rows {
			f := strings.SplitN(r, "\t", 3)
			mark := " "
			if f[0] == cur && ref == "refs/heads" {
				mark = "*"
			}
			fmt.Fprintf(&b, "%s %s  %s  %s\n", mark, f[0], f[1], f[2])
		}
	}
	section("локальные:", "refs/heads")
	b.WriteString("\n")
	section("удалённые:", "refs/remotes")
	return b.String()
}

func history(in Input, snap string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "История коммитов: до %d последних на каждую ветку, новые сверху, формат «хэш тема». Читай, когда нужна история.\n%s\n", MaxBranchCommits, snap)
	refs := lines(git(in.Root, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes"))
	if len(refs) == 0 {
		b.WriteString("\nкоммитов нет\n")
		return b.String()
	}
	for _, r := range refs {
		short := strings.TrimPrefix(strings.TrimPrefix(r, "refs/heads/"), "refs/")
		if strings.HasSuffix(r, "/HEAD") {
			continue
		}
		total := git(in.Root, "rev-list", "--count", r)
		fmt.Fprintf(&b, "\n## %s (всего %s)\n", short, total)
		for _, l := range lines(git(in.Root, "log", "-n", fmt.Sprint(MaxBranchCommits), "--format=%h %s", r)) {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// Paths — файлы репозитория по версии git: отслеживаемые и новые без игнорируемых; удалённые с диска пропускаются.
func Paths(root string) []string {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate").Output()
	if err != nil {
		return nil
	}
	var ps []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, p)); err != nil {
			continue // удалён с диска, но ещё в индексе
		}
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

func paths(in Input) []string { return Paths(in.Root) }

func capped(b *strings.Builder, rows []string) {
	n := len(rows)
	if n > MaxTreeLines {
		rows = rows[:MaxTreeLines]
	}
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	if n > MaxTreeLines {
		fmt.Fprintf(b, "… скрыто строк: %d\n", n-MaxTreeLines)
	}
}

func tree(in Input, snap string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Файлы репозитория деревом: отслеживаемые и новые, без игнорируемых; не больше %d строк. Читай, когда нужно найти файл.\n%s\n\n", MaxTreeLines, snap)
	var rows []string
	var prev []string
	for _, p := range paths(in) {
		parts := strings.Split(p, "/")
		dirs := parts[:len(parts)-1]
		common := 0
		for common < len(dirs) && common < len(prev) && dirs[common] == prev[common] {
			common++
		}
		for i := common; i < len(dirs); i++ {
			rows = append(rows, strings.Repeat("  ", i)+dirs[i]+"/")
		}
		rows = append(rows, strings.Repeat("  ", len(dirs))+parts[len(parts)-1])
		prev = dirs
	}
	if len(rows) == 0 {
		b.WriteString("файлов нет\n")
		return b.String()
	}
	capped(&b, rows)
	return b.String()
}

func graph(in Input, snap string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Дерево веток: граф коммитов всех веток с указателями (HEAD, ветки, remote, теги), новые сверху; не больше %d строк. Читай, когда нужно понять, как ветки расходятся и сливаются.\n%s\n\n", MaxTreeLines, snap)
	g := git(in.Root, "log", "--graph", "--all", "--date-order", "--decorate=short", "--format=%h%d %s", "-n", fmt.Sprint(MaxTreeLines))
	if g == "" {
		b.WriteString("коммитов нет\n")
		return b.String()
	}
	rows := lines(g)
	total := lines(git(in.Root, "rev-list", "--all", "--count"))
	capped(&b, rows)
	if len(total) == 1 && total[0] != "" {
		var n int
		fmt.Sscanf(total[0], "%d", &n)
		if n > MaxTreeLines {
			fmt.Fprintf(&b, "… скрыто коммитов: %d\n", n-MaxTreeLines)
		}
	}
	return b.String()
}

func stages(in Input, snap string) string {
	var b strings.Builder
	b.WriteString("Записи .sky/stages/history/: применённые патчи и откаты, от старых к новым. Читай, когда нужно знать, что уже применялось.\n" + snap + "\n\n")
	es, _ := apply.Entries(in.Stages)
	if len(es) == 0 {
		b.WriteString("записей нет\n")
		return b.String()
	}
	for _, e := range es {
		commit := ""
		if e.Commit != "" {
			commit = fmt.Sprintf("  коммит %.7s", e.Commit)
		}
		fmt.Fprintf(&b, "%04d  %s  файлов: %d%s\n", e.Num, e.Source, e.Files, commit)
	}
	return b.String()
}
