package pack

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func write(t *testing.T, root, rel string, body []byte) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T) string {
	d := t.TempDir()
	sh(t, d, "init", "-q", "-b", "main")
	write(t, d, ".gitignore", []byte(".sky/\n.env\n*.db\n"))
	write(t, d, "main.go", []byte("package main\n"))
	write(t, d, "docs/A.md", []byte("a\n"))
	write(t, d, ".env", []byte("SECRET=1\n"))
	write(t, d, "x.db", []byte("db"))
	write(t, d, ".sky/clouds/n.md", []byte("заметки\n"))
	write(t, d, ".sky/sky.conf", []byte("scope = [./]\n"))
	write(t, d, ".sky/readme/readme.md", []byte("r\n"))
	write(t, d, ".sky/foryou/status.md", []byte("s\n"))
	write(t, d, ".sky/stages/0001-x/before.zip", []byte("z"))
	sh(t, d, "add", "-A")
	sh(t, d, "commit", "-q", "-m", "chore: base")
	return d
}

func names(t *testing.T, zp string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(zp)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func opts(d string, k Kind) Options {
	return Options{Root: d, Backups: filepath.Join(d, ".sky", "stages", "backups"), Kind: k, ForChat: k == Up}
}

func TestUpContents(t *testing.T) {
	d := project(t)
	write(t, d, "new.txt", []byte("новый\n")) // не в git, не игнорируется
	rep, err := Make(opts(d, Up))
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, rep.Zip)
	for _, want := range []string{"main.go", "docs/A.md", ".gitignore", "new.txt", ".sky/clouds/n.md", ".sky/sky.conf", ".sky/readme/readme.md", ".sky/foryou/status.md", ".sky/foryou/archive.md"} {
		if _, ok := got[want]; !ok {
			t.Errorf("в архиве нет %s", want)
		}
	}
	for _, bad := range []string{".env", "x.db", ".sky/stages/0001-x/before.zip"} {
		if _, ok := got[bad]; ok {
			t.Errorf("в архиве не должно быть %s", bad)
		}
	}
	if !strings.Contains(got[".sky/foryou/archive.md"], "файлов: 8") {
		t.Errorf("archive.md:\n%s", got[".sky/foryou/archive.md"])
	}
	if !strings.HasSuffix(rep.Zip, "-up.zip") {
		t.Errorf("имя: %s", rep.Zip)
	}
}

func TestBeforeSnapshotHasNoGeneratedReadme(t *testing.T) {
	d := project(t)
	o := opts(d, Before)
	o.Label = "patch-a.zip"
	rep, err := Make(o)
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, rep.Zip)
	if _, ok := got[".sky/readme/readme.md"]; ok {
		t.Error("в снимке перед патчем readme/ не нужен")
	}
	if _, ok := got["main.go"]; !ok {
		t.Error("нет main.go")
	}
	if !strings.HasSuffix(rep.Zip, "-before-patch-a.zip") {
		t.Errorf("имя: %s", rep.Zip)
	}
}

func TestBigFileSkippedWithHead(t *testing.T) {
	d := project(t)
	big := []byte(strings.Repeat("строка данных\n", MaxFile/10))
	write(t, d, "data/big.csv", big)
	write(t, d, "data/blob.bin", append([]byte{0, 1, 2}, make([]byte, MaxFile)...))
	rep, err := Make(opts(d, Up))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := names(t, rep.Zip)["data/big.csv"]; ok {
		t.Error("большой файл не должен попасть в архив")
	}
	md := rep.Markdown
	for _, want := range []string{"data/big.csv: больше 5 МБ", "текст", "    | строка данных", "data/blob.bin: больше 5 МБ", "бинарный"} {
		if !strings.Contains(md, want) {
			t.Errorf("в archive.md нет %q:\n%s", want, md)
		}
	}
	if strings.Count(md, "    | ") != headLines {
		t.Errorf("строк образца: %d", strings.Count(md, "    | "))
	}
}

func TestSensitiveNotIgnoredIsIncludedWithWarning(t *testing.T) {
	d := project(t)
	write(t, d, "certs/server.pem", []byte("pem"))
	rep, err := Make(opts(d, Up))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := names(t, rep.Zip)["certs/server.pem"]; !ok {
		t.Error("файл, не закрытый .gitignore, берётся")
	}
	if len(rep.Sensitive) != 1 || rep.Sensitive[0] != "certs/server.pem" || !strings.Contains(rep.Markdown, "certs/server.pem") {
		t.Errorf("предупреждение: %v\n%s", rep.Sensitive, rep.Markdown)
	}
}

func TestSymlinkSkipped(t *testing.T) {
	d := project(t)
	os.Symlink("main.go", filepath.Join(d, "link.go"))
	rep, _ := Make(opts(d, Up))
	if _, ok := names(t, rep.Zip)["link.go"]; ok {
		t.Error("ссылка не должна попасть в архив")
	}
	if !strings.Contains(rep.Markdown, "link.go: не обычный файл") {
		t.Errorf("%s", rep.Markdown)
	}
}

func TestPruneKeepsLimitBefore(t *testing.T) {
	d := project(t)
	o := opts(d, Before)
	o.Label = "patch-x.zip"
	for i := 0; i < Keep+3; i++ {
		if _, err := Make(o); err != nil {
			t.Fatal(err)
		}
	}
	es, _ := os.ReadDir(o.Backups)
	if len(es) != Keep {
		t.Errorf("снимков: %d", len(es))
	}
	if !strings.HasPrefix(es[len(es)-1].Name(), "0019-") {
		t.Errorf("последний: %s", es[len(es)-1].Name())
	}
}

func TestUpIsTransit(t *testing.T) {
	d := project(t)
	o := opts(d, Up)
	o.Backups = filepath.Join(t.TempDir(), "sky-up")
	r1, err := Make(o)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Make(o)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Zip != r2.Zip {
		t.Errorf("up должен перезаписывать один файл: %s и %s", r1.Zip, r2.Zip)
	}
	es, _ := os.ReadDir(o.Backups)
	if len(es) != 1 {
		t.Errorf("файлов: %d", len(es))
	}
	if _, err := os.Stat(filepath.Join(d, ".sky", "stages", "backups")); err == nil {
		t.Error("up не должен создавать записей в backups/")
	}
}
