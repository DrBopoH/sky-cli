// Package pr — удалённая часть, этап R1 (регламент, 3.1.4): локальная ветка фичи уходит в удалённую область
// pull request'ом. sky пушит только саму wip-ветку (без force) и открывает PR через gh: в dev, а исправление — в ветку
// сборки, от которой оно отведено (3.6.5). Версию в PR не вписывает — её ставит слияние (3.5.6).
// Цель ветки (модуль, цепочка, зерно, сборка) уходит в конец описания PR машиночитаемым блоком.
package pr

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/named"
)

const (
	Remote = "origin"
	Dev    = "dev"
)

// Options — параметры одного вызова.
type Options struct {
	Root    string
	Branch  string   // локальная ветка фичи
	Summary string   // заголовок PR и будущее резюме именованного коммита
	Kind    string   // seed, module или fix (цель ветки)
	Module  string   // для module
	Chain   []string // для module
	Base    string   // ветка, в которую открывается PR; пусто — dev (для fix — ветка сборки, от которой отведена фича)
	Draft   bool
	Dry     bool
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines []string
	URL   string
	Done  bool
}

// Body — описание PR: резюме и блок с целью ветки, который читает слияние.
func Body(o Options) string {
	var b strings.Builder
	b.WriteString(o.Summary + "\n\n---\n")
	fmt.Fprintf(&b, "sky-branch: %s\nsky-target: %s\n", o.Branch, o.Kind)
	switch o.Kind {
	case "module":
		fmt.Fprintf(&b, "sky-module: %s\nsky-chain: %s\n", o.Module, strings.Join(o.Chain, ","))
	case "fix":
		fmt.Fprintf(&b, "sky-build: %s\n", o.Base)
	}
	return b.String()
}

// Run выполняет R1 по параметрам.
func Run(o Options) (Result, error) {
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	if err := named.CheckLine("резюме", o.Summary); err != nil {
		return res, err
	}
	switch o.Kind {
	case "seed", "module":
	case "fix":
		if o.Base == "" {
			return res, fmt.Errorf("у ветки исправления %s не записана ветка сборки: задай --base build/ММДД-ЧЧММ", o.Branch)
		}
	default:
		return res, fmt.Errorf("у ветки %s нет цели: заведи её командой sky feature", o.Branch)
	}
	if o.Base == "" {
		o.Base = Dev
	}
	root := o.Root
	if !gitx.OK(root, "remote", "get-url", Remote) {
		return res, fmt.Errorf("нет удалённого репозитория %s: добавь его (git remote add %s <url>); пока его нет, фичи вливает локальный sky land", Remote, Remote)
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return res, fmt.Errorf("не найдена команда gh (GitHub CLI): установи её и войди (gh auth login)")
	}
	if d := gitx.Dirty(root); d != "" {
		return res, fmt.Errorf("в отслеживаемых файлах есть незакоммиченные правки: закоммить (sky commit) или убери\n%s", d)
	}
	base := Remote + "/" + o.Base
	if !gitx.OK(root, "rev-parse", "--verify", "-q", "refs/remotes/"+base) {
		base = o.Base
	}
	if !gitx.OK(root, "rev-parse", "--verify", "-q", base) {
		return res, fmt.Errorf("нет ветки %s: PR открывать не от чего", o.Base)
	}
	if strings.TrimSpace(gitx.Try(root, "rev-list", "--count", base+".."+o.Branch)) == "0" {
		return res, fmt.Errorf("в %s нет коммитов, которых нет в %s", o.Branch, base)
	}
	url := existing(root, o.Branch)
	say("PR: %s → %s, «%s»", o.Branch, o.Base, o.Summary)
	if url != "" {
		say("  PR уже открыт: %s (будет обновлён пушем)", url)
	}
	if o.Dry {
		say("  push %s %s, затем gh pr create (описание — резюме и блок sky-target)", Remote, o.Branch)
		return res, nil
	}
	if out, err := gitx.Run(root, "push", "-u", Remote, o.Branch); err != nil {
		return res, fmt.Errorf("push не удался (без force; чужую историю sky не перезаписывает): %v", err)
	} else if s := strings.TrimSpace(out); s != "" {
		say("%s", s)
	}
	if url == "" {
		args := []string{"pr", "create", "--base", o.Base, "--head", o.Branch, "--title", o.Summary, "--body", Body(o)}
		if o.Draft {
			args = append(args, "--draft")
		}
		out, err := gh(root, args...)
		if err != nil {
			return res, fmt.Errorf("ветка запушена, но PR не открыт: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		url = strings.TrimSpace(lines[len(lines)-1])
	}
	res.URL, res.Done = url, true
	if _, err := gitx.Run(root, "config", "branch."+o.Branch+".sky-pr", url); err != nil {
		say("⚠ адрес PR не записан в настройки ветки: %v", err)
	}
	say("✓ %s", url)
	return res, nil
}

// existing — адрес открытого PR этой ветки или пустая строка.
func existing(root, branch string) string {
	out, err := gh(root, "pr", "list", "--head", branch, "--state", "open", "--json", "url", "--jq", ".[0].url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gh(root string, args ...string) (string, error) {
	c := exec.Command("gh", args...)
	c.Dir = root
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return out.String(), fmt.Errorf("gh %s: %v: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
