// Package cli — диспетчер команд sky.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/conf"
	"github.com/DrBopoH/sky-cli/internal/readme"
	"github.com/DrBopoH/sky-cli/internal/repo"
	"github.com/DrBopoH/sky-cli/internal/version"
)

type Env struct {
	Cwd string
	Out io.Writer
	Err io.Writer
}

type Command struct {
	Name      string
	Usage     string
	Summary   string
	Help      string // подробная справка: sky <команда> --help, sky help <команда>
	NeedsInit bool   // нужна готовая .sky/; перед запуском sky заново проверяет .sky/readme/
	Run       func(a *App, env Env, args []string) error
}

type App struct {
	Commands   []Command
	Plan       apply.Planner  // как читать патч; подставляет точка входа
	Operations []readme.Entry // операции патча для readme.md; их знает адаптер патча, собирает точка входа
}

// New собирает консоль. Что делать с патчем и какие операции он знает, консоль не знает: это даёт точка входа.
func New(plan apply.Planner, operations []readme.Entry) *App {
	return &App{
		Commands:   []Command{initCommand(), applyCommand(), commitCommand(), rollbackCommand(), lookCommand(), upCommand(), landCommand(), buildCommand(), fixCommand()},
		Plan:       plan,
		Operations: operations,
	}
}

// Entries — команды для readme.md: реестр плюс встроенные help и version.
func (a *App) Entries() []readme.Entry {
	out := make([]readme.Entry, 0, len(a.Commands)+2)
	for _, c := range a.Commands {
		out = append(out, readme.Entry{Usage: c.Usage, Summary: c.Summary})
	}
	return append(out,
		readme.Entry{Usage: "sky help", Summary: "список команд"},
		readme.Entry{Usage: "sky --version", Summary: "версия sky"},
	)
}

// Heal приводит .sky/readme/ к генерируемому набору: недостающее создаёт, изменённое и лишнее убирает.
func (a *App) Heal(l repo.Layout) ([]repo.Change, error) {
	files := readme.Files(a.Entries(), a.Operations, conf.Render(conf.Default()))
	gen := make([]repo.Generated, len(files))
	for i, f := range files {
		gen[i] = repo.Generated{Name: f.Name, Content: f.Content}
	}
	return l.SyncReadme(gen)
}

func (a *App) find(name string) *Command {
	for i := range a.Commands {
		if a.Commands[i].Name == name {
			return &a.Commands[i]
		}
	}
	return nil
}

func (a *App) Run(cwd string, args []string, out, errw io.Writer) int {
	env := Env{Cwd: cwd, Out: out, Err: errw}
	if len(args) == 0 {
		a.help(env)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) > 1 && args[0] == "help" {
			return a.commandHelp(env, args[1])
		}
		a.help(env)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(out, "sky %s\n", version.String())
		return 0
	}
	c := a.find(args[0])
	if c == nil {
		fmt.Fprintf(errw, "✗ нет команды «%s». sky help — список\n", args[0])
		return 2
	}
	if len(args) > 1 && (args[1] == "-h" || args[1] == "--help") {
		return a.commandHelp(env, c.Name)
	}
	if c.NeedsInit {
		l, err := repo.Find(cwd)
		if err == nil && !l.Initialized() {
			err = fmt.Errorf("нет .sky/ — сначала sky init")
		}
		if err == nil {
			_, err = a.Heal(l)
		}
		if err != nil {
			fmt.Fprintf(errw, "✗ %v\n", err)
			return 1
		}
	}
	if err := c.Run(a, env, args[1:]); err != nil {
		fmt.Fprintf(errw, "✗ %v\n", err)
		return 1
	}
	return 0
}

func (a *App) help(env Env) {
	var b strings.Builder
	b.WriteString("sky — мост между чатом с Claude и репозиторием\n\n")
	for _, e := range a.Entries() {
		fmt.Fprintf(&b, "  %-34s %s\n", e.Usage, e.Summary)
	}
	fmt.Fprint(env.Out, b.String())
}

// commandHelp печатает подробную справку команды; без неё — её строку из общего списка.
func (a *App) commandHelp(env Env, name string) int {
	c := a.find(name)
	if c == nil {
		fmt.Fprintf(env.Err, "✗ нет команды «%s». sky help — список\n", name)
		return 2
	}
	if c.Help == "" {
		fmt.Fprintf(env.Out, "%s\n\n%s\n", c.Usage, c.Summary)
		return 0
	}
	fmt.Fprint(env.Out, c.Help)
	return 0
}
