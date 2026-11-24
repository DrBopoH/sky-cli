package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/pack"
	"github.com/DrBopoH/sky-cli/internal/repo"
)

func upCommand() Command {
	return Command{
		Name:      "up",
		Usage:     "sky up",
		Summary:   summaryUp,
		NeedsInit: true,
		Run:       withLook(runUp, false),
	}
}

func runUp(a *App, env Env, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("sky up без аргументов")
	}
	l, err := repo.Find(env.Cwd)
	if err != nil {
		return err
	}
	if _, err := refreshForyou(l); err != nil {
		return err
	}
	rep, err := pack.Make(pack.Options{Root: l.Root, Backups: filepath.Join(os.TempDir(), "sky-up"), Kind: pack.Up, ForChat: true})
	if err != nil {
		return err
	}
	// archive.md остаётся в foryou, пока его не заменит следующий up
	if err := l.WriteForyouFile("archive.md", []byte(rep.Markdown)); err != nil {
		fmt.Fprintf(env.Out, "⚠ archive.md не записан: %v\n", err)
	}
	fmt.Fprintf(env.Out, "✓ %s (%s, файлов: %d)\n", rep.Zip, pack.HumanSize(rep.Size), rep.Files)
	for _, s := range rep.Sensitive {
		fmt.Fprintf(env.Out, "⚠ в архиве файл с именем базы или ключа, не закрытый .gitignore: %s\n", s)
	}
	if n := len(rep.Skipped); n > 0 {
		fmt.Fprintf(env.Out, "· пропущено файлов: %d (список в foryou/archive.md)\n", n)
	}
	if rep.OverWarn {
		fmt.Fprintf(env.Out, "⚠ архив больше %s: вложение в чат может не пройти\n", pack.HumanSize(pack.WarnTotal))
	}
	if clip(rep.Zip) {
		fmt.Fprintln(env.Out, "📋 в буфере обмена: вставь Ctrl+V в чат")
	} else {
		fmt.Fprintln(env.Out, "· буфер обмена недоступен (нужен wl-copy или xclip), файл по пути выше")
	}
	return nil
}

// SKY_NO_CLIP=1 отключает работу с буфером (для тестов).
// clip кладёт файл в буфер обмена как file://-ссылку (text/uri-list), которую понимают поля вложений.
func clip(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	data := "file://" + strings.ReplaceAll(abs, " ", "%20") + "\n"
	for _, c := range [][]string{
		{"wl-copy", "-t", "text/uri-list"},
		{"xclip", "-selection", "clipboard", "-t", "text/uri-list"},
	} {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(data)
		if os.Getenv("SKY_NO_CLIP") == "" && cmd.Run() == nil {
			return true
		}
	}
	return false
}
