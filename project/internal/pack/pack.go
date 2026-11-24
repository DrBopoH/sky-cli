// Package pack собирает зип проекта: для чата (sky up) и полный снимок перед патчем. Отбор файлов отдан git.
package pack

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/guard"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DrBopoH/sky-cli/internal/gitx"
)

//go:embed text/archive.txt
var preArchiveRaw string

var preArchive = strings.TrimSpace(preArchiveRaw)

const (
	MaxFile   = 5 << 20  // больше — файл в архив не кладётся
	WarnTotal = 25 << 20 // больше — предупреждение: вложение в чат может не пройти
	Keep      = 16       // сколько снимков before хранить в backups/
	headFiles = 20       // у скольких пропущенных файлов показывать начало
	headLines = 5
	headWidth = 160
)

// Kind — вид архива в backups/.
type Kind string

const (
	Up     Kind = "up"
	Before Kind = "before"
)

type skip struct {
	Path, Reason string
	Size         int64
	Head         []string
	Binary       bool
}

// Report — что вошло в архив и что нет.
type Report struct {
	Zip       string
	Files     int
	Size      int64 // размер зипа
	Raw       int64 // сумма размеров файлов
	Skipped   []skip
	Sensitive []string // вошли, но имя похоже на базу или ключ и не закрыто .gitignore
	Markdown  string   // содержимое archive.md
	OverWarn  bool
}

// Options задаёт, что класть в архив.
type Options struct {
	Root    string
	Backups string // куда класть зип: .sky/stages/backups для before, временная папка для up
	Kind    Kind
	Label   string // для before: имя патча
	ForChat bool   // true: добавить .sky/readme/ и .sky/foryou/
}

var sky = []string{".sky/clouds", ".sky/sky.conf"}

func nextNum(dir string) int {
	es, _ := os.ReadDir(dir)
	n := 0
	for _, e := range es {
		var k int
		if _, err := fmt.Sscanf(e.Name(), "%04d-", &k); err == nil && k > n {
			n = k
		}
	}
	return n + 1
}

var safe = strings.NewReplacer("/", "_", " ", "_", "\\", "_")

func sniff(abs string) (binary bool, head []string) {
	f, err := os.Open(abs)
	if err != nil {
		return true, nil
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if bytes.IndexByte(buf, 0) >= 0 {
		return true, nil
	}
	for _, l := range strings.Split(string(buf), "\n") {
		if len(head) == headLines {
			break
		}
		if len(l) > headWidth {
			l = l[:headWidth] + "…"
		}
		head = append(head, l)
	}
	return false, head
}

// HumanSize — размер для людей: Б, КБ или МБ.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f КБ", float64(n)/1024)
	}
	return fmt.Sprintf("%d Б", n)
}

// sky-часть: файлы из .sky/, которых нет в git (папка в .gitignore).
func skyFiles(root string, forChat bool) []string {
	dirs := append([]string{}, sky...)
	if forChat {
		dirs = append(dirs, ".sky/readme", ".sky/foryou")
	}
	var out []string
	for _, d := range dirs {
		filepath.Walk(filepath.Join(root, filepath.FromSlash(d)), func(p string, fi os.FileInfo, err error) error {
			if err == nil && fi.Mode().IsRegular() {
				rel, _ := filepath.Rel(root, p)
				out = append(out, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	return out
}

// Make создаёт архив в backups/ и возвращает отчёт. archive.md внутри зипа описывает сам архив.
func Make(o Options) (Report, error) {
	if err := os.MkdirAll(o.Backups, 0o755); err != nil {
		return Report{}, err
	}
	var rep Report
	seen := map[string]bool{}
	var all []string
	for _, p := range append(gitx.Present(o.Root), skyFiles(o.Root, o.ForChat)...) {
		if !seen[p] && p != ".sky/foryou/archive.md" {
			seen[p] = true
			all = append(all, p)
		}
	}
	sort.Strings(all)

	type item struct {
		path string
		mode os.FileMode
		data []byte
	}
	var items []item
	for _, p := range all {
		abs := filepath.Join(o.Root, filepath.FromSlash(p))
		fi, err := os.Lstat(abs)
		switch {
		case err != nil:
			continue
		case !fi.Mode().IsRegular():
			rep.Skipped = append(rep.Skipped, skip{Path: p, Reason: "не обычный файл (ссылка или подмодуль)"})
			continue
		case fi.Size() > MaxFile:
			bin, head := sniff(abs)
			rep.Skipped = append(rep.Skipped, skip{Path: p, Reason: "больше 5 МБ", Size: fi.Size(), Binary: bin, Head: head})
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			rep.Skipped = append(rep.Skipped, skip{Path: p, Reason: "не прочитался: " + err.Error()})
			continue
		}
		if guard.IsSensitive(p) {
			rep.Sensitive = append(rep.Sensitive, p)
		}
		items = append(items, item{p, fi.Mode().Perm(), b})
		rep.Raw += int64(len(b))
	}
	rep.Files = len(items)
	rep.Markdown = markdown(o, rep, gitx.Head(o.Root))

	// up — транзит для чата: один файл на проект, перезаписывается, в историю не попадает
	name := safe.Replace(filepath.Base(o.Root)) + "-up"
	if o.Kind != Up {
		name = fmt.Sprintf("%04d-%s-%s", nextNum(o.Backups), time.Now().Format("20060102-150405"), o.Kind)
		if o.Label != "" {
			name += "-" + safe.Replace(strings.TrimSuffix(o.Label, filepath.Ext(o.Label)))
		}
	}
	rep.Zip = filepath.Join(o.Backups, name+".zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(p string, mode os.FileMode, data []byte) error {
		h := &zip.FileHeader{Name: p, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	for _, it := range items {
		if err := add(it.path, it.mode, it.data); err != nil {
			return rep, err
		}
	}
	if err := add(".sky/foryou/archive.md", 0o644, []byte(rep.Markdown)); err != nil {
		return rep, err
	}
	if err := zw.Close(); err != nil {
		return rep, err
	}
	if err := os.WriteFile(rep.Zip, buf.Bytes(), 0o644); err != nil {
		return rep, err
	}
	rep.Size = int64(buf.Len())
	rep.OverWarn = rep.Size > WarnTotal
	if o.Kind != Up {
		prune(o.Backups, o.Kind)
	}
	return rep, nil
}

func kindOf(name string) Kind {
	switch {
	case strings.Contains(name, "-up"):
		return Up
	case strings.Contains(name, "-before"):
		return Before
	}
	return ""
}

// prune оставляет Keep последних архивов вида kind.
func prune(dir string, kind Kind) {
	es, _ := os.ReadDir(dir)
	var names []string
	for _, e := range es {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".zip") && kindOf(e.Name()) == kind {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > Keep {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

func markdown(o Options, r Report, head string) string {
	var b strings.Builder
	what := "архива sky up (для чата)"
	if o.Kind == Before {
		what = "снимка перед патчем"
	}
	fmt.Fprintf(&b, preArchive+"\n", what)
	if head != "" {
		fmt.Fprintf(&b, "Снимок при HEAD %s.\n", head)
	} else {
		b.WriteString("Снимок: коммитов ещё нет.\n")
	}
	fmt.Fprintf(&b, "\nфайлов: %d, исходный размер: %s\n", r.Files, HumanSize(r.Raw))
	b.WriteString("отбор файлов: git (отслеживаемые и новые без игнорируемых) и из .sky/: clouds/, sky.conf")
	if o.ForChat {
		b.WriteString(", readme/, foryou/")
	}
	b.WriteString("\n")
	if len(r.Sensitive) > 0 {
		b.WriteString("\nвошли файлы с именами баз или ключей, не закрытые .gitignore:\n")
		for _, p := range r.Sensitive {
			b.WriteString("  " + p + "\n")
		}
	}
	b.WriteString("\nпропущено:")
	if len(r.Skipped) == 0 {
		b.WriteString(" нет\n")
		return b.String()
	}
	b.WriteString("\n")
	for i, s := range r.Skipped {
		kind := "текст"
		if s.Binary {
			kind = "бинарный"
		}
		if s.Size == 0 {
			fmt.Fprintf(&b, "  %s: %s\n", s.Path, s.Reason)
			continue
		}
		fmt.Fprintf(&b, "  %s: %s, %s, %s\n", s.Path, s.Reason, HumanSize(s.Size), kind)
		if i < headFiles {
			for _, l := range s.Head {
				fmt.Fprintf(&b, "    | %s\n", l)
			}
		}
	}
	return b.String()
}

// File — файл из снимка.
type File struct {
	Data []byte
	Exec bool
}

// ReadSnapshot читает зип снимка в память (пути с «/»).
func ReadSnapshot(zipPath string) (map[string]File, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := map[string]File{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
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
		out[f.Name] = File{b, f.Mode()&0o111 != 0}
	}
	return out, nil
}
