// Package sync — удалённая часть, этап R2: подтягивает общие ветки из origin (main, dev, ветки сборок и модулей),
// чтобы land, build и modules работали по реальной удалённой области. Удалённая область — источник: sky только
// догоняет её локальными ветками (создаёт недостающие, продвигает отстающие без слияний). В обратную сторону работает только
// Publish: отправляет общие ветки, вершины которых изменились после локальной операции (land, fix), без force.
// Ветку, где у автора есть свои коммиты или история разошлась, sky не трогает, а называет. wip-ветки не затрагиваются.
package sync

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/modules"
	"github.com/DrBopoH/sky-cli/internal/vhist"
)

const (
	Remote = "origin"
	Main   = "main"
	Dev    = "dev"
)

// Options — параметры одного вызова.
type Options struct {
	Root string
	Dry  bool // без fetch и без изменений: только сверка по уже известным удалённым веткам
}

// State — как локальная ветка соотносится с удалённой.
type State string

const (
	Same     State = "same"     // совпадают
	New      State = "new"      // локальной нет: создана от удалённой
	Forward  State = "forward"  // локальная отстаёт: продвинута до удалённой
	Ahead    State = "ahead"    // у локальной есть свои коммиты: не тронута
	Diverged State = "diverged" // истории разошлись: не тронута
	Blocked  State = "blocked"  // нужно продвинуть, но мешает грязное дерево на этой ветке: не тронута
)

// Item — одна общая ветка.
type Item struct {
	Name  string
	State State
	N     int // сколько коммитов добавлено (New, Forward), не хватает удалённой (Ahead — свои)
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines []string
	Items []Item
}

// Shared сообщает, общая ли это ветка удалённой области: main, dev, сборка или ветка модуля.
func Shared(name string) bool {
	if name == Main || name == Dev || strings.HasPrefix(name, vhist.Prefix) {
		return true
	}
	_, _, ok := modules.ParseBranch(name)
	return ok
}

// Run выполняет R2 по параметрам.
func Run(o Options) (Result, error) {
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	root := o.Root
	if !gitx.OK(root, "remote", "get-url", Remote) {
		return res, fmt.Errorf("нет удалённого репозитория %s: добавь его (git remote add %s <url>); пока его нет, всё идёт локально", Remote, Remote)
	}
	if o.Dry {
		say("· --dry: без fetch, по последним известным веткам %s", Remote)
	} else if out, err := gitx.Run(root, "fetch", "--prune", "--quiet", Remote); err != nil {
		return res, fmt.Errorf("fetch не удался: %v", err)
	} else if s := strings.TrimSpace(out); s != "" {
		say("%s", s)
	}
	cur := strings.TrimSpace(gitx.Try(root, "symbolic-ref", "-q", "--short", "HEAD"))
	prefix := Remote + "/"
	for _, ref := range strings.Split(strings.TrimSpace(gitx.Try(root, "for-each-ref", "--format=%(refname:short)", "refs/remotes/"+Remote+"/")), "\n") {
		name := strings.TrimPrefix(ref, prefix)
		if name == ref || name == "HEAD" || !Shared(name) {
			continue
		}
		it := Item{Name: name}
		local := gitx.OK(root, "rev-parse", "--verify", "-q", "refs/heads/"+name)
		switch {
		case !local:
			it.State, it.N = New, count(root, "", ref)
		case same(root, name, ref):
			it.State = Same
		case gitx.OK(root, "merge-base", "--is-ancestor", "refs/heads/"+name, ref):
			it.State, it.N = Forward, count(root, "refs/heads/"+name, ref)
		case gitx.OK(root, "merge-base", "--is-ancestor", ref, "refs/heads/"+name):
			it.State, it.N = Ahead, count(root, ref, "refs/heads/"+name)
		default:
			it.State = Diverged
		}
		if it.State == Forward && name == cur && gitx.Dirty(root) != "" {
			it.State = Blocked
		}
		if !o.Dry {
			if err := apply(root, it, ref, cur); err != nil {
				return res, err
			}
		}
		res.Items = append(res.Items, it)
		say("  %-24s %s", name, describe(it, o.Dry))
	}
	if len(res.Items) == 0 {
		say("общих веток на %s пока нет", Remote)
	}
	return res, nil
}

func same(root, name, ref string) bool {
	return strings.TrimSpace(gitx.Try(root, "rev-parse", "refs/heads/"+name)) == strings.TrimSpace(gitx.Try(root, "rev-parse", ref))
}

// count — число коммитов в to, которых нет в from (from пусто — все).
func count(root, from, to string) int {
	rng := to
	if from != "" {
		rng = from + ".." + to
	}
	var n int
	fmt.Sscan(strings.TrimSpace(gitx.Try(root, "rev-list", "--count", rng)), &n)
	return n
}

func apply(root string, it Item, ref, cur string) error {
	switch it.State {
	case New:
		_, err := gitx.Run(root, "branch", it.Name, ref)
		return err
	case Forward:
		if it.Name == cur {
			_, err := gitx.Run(root, "merge", "--ff-only", "-q", ref)
			return err
		}
		old := strings.TrimSpace(gitx.Try(root, "rev-parse", "refs/heads/"+it.Name))
		_, err := gitx.Run(root, "update-ref", "-m", "sky sync", "refs/heads/"+it.Name, strings.TrimSpace(gitx.Try(root, "rev-parse", ref)), old)
		return err
	}
	return nil
}

func describe(it Item, dry bool) string {
	will := func(done, plan string) string {
		if dry {
			return plan
		}
		return done
	}
	switch it.State {
	case Same:
		return "совпадает"
	case New:
		return will(fmt.Sprintf("создана локально (%d коммитов)", it.N), fmt.Sprintf("будет создана локально (%d коммитов)", it.N))
	case Forward:
		return will(fmt.Sprintf("продвинута на %d", it.N), fmt.Sprintf("будет продвинута на %d", it.N))
	case Ahead:
		return fmt.Sprintf("⚠ у неё есть свои коммиты (%d), не тронута: в удалённую область они идут только через PR (sky pr)", it.N)
	case Blocked:
		return "⚠ отстаёт, но на ней незакоммиченные правки: не тронута"
	}
	return "⚠ истории разошлись, не тронута: реши вручную"
}

// Tips — вершины общих веток (имя → коммит) для сравнения до и после локальной операции.
func Tips(root string) map[string]string {
	out := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSpace(gitx.Try(root, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/")), "\n") {
		n, h, ok := strings.Cut(ln, " ")
		if ok && Shared(n) {
			out[n] = h
		}
	}
	return out
}

// HasRemote сообщает, настроен ли origin.
func HasRemote(root string) bool { return gitx.OK(root, "remote", "get-url", Remote) }

// Publish отправляет в origin общие ветки, изменившиеся с момента снимка before (без force). Возвращает ушедшие ветки;
// при ошибке — те, что успели уйти, и ошибку для первой неудавшейся. Без origin ничего не делает.
func Publish(root string, before map[string]string) ([]string, error) {
	if !HasRemote(root) {
		return nil, nil
	}
	var names, pushed []string
	for n, h := range Tips(root) {
		if before[n] != h {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if _, err := gitx.Run(root, "push", "-u", Remote, n); err != nil {
			return pushed, fmt.Errorf("ветка %s: push не удался (повтори git push -u %s %s): %v", n, Remote, n, err)
		}
		pushed = append(pushed, n)
	}
	return pushed, nil
}
