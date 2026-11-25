// Package graph читает реестр модулей проекта (docs/PROJECT-GRAPH.md): модули, зависимости, цепочки от зерна.
package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Path — где реестр лежит в репозитории.
const Path = "docs/PROJECT-GRAPH.md"

// Graph — модули и их прямые зависимости.
type Graph struct {
	Deps  map[string][]string
	Order []string
}

func cells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func clean(s string) string { return strings.TrimSpace(strings.Trim(s, "` ")) }

// Parse берёт первую таблицу, у которой первая колонка «Модуль», а последняя содержит «Зависит».
func Parse(md string) (Graph, error) {
	lines := strings.Split(md, "\n")
	g := Graph{Deps: map[string][]string{}}
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			continue
		}
		h := cells(lines[i])
		if len(h) < 2 || strings.ToLower(h[0]) != "модуль" || !strings.Contains(strings.ToLower(h[len(h)-1]), "завис") {
			continue
		}
		for j := i + 2; j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|"); j++ {
			c := cells(lines[j])
			if len(c) < len(h) {
				continue
			}
			name := clean(c[0])
			if name == "" {
				continue
			}
			if _, dup := g.Deps[name]; dup {
				return Graph{}, fmt.Errorf("модуль «%s» записан дважды", name)
			}
			var deps []string
			for _, d := range strings.Split(c[len(h)-1], ",") {
				d = clean(d)
				if d == "" || d == "—" || d == "-" || d == "–" {
					continue
				}
				deps = append(deps, d)
			}
			g.Deps[name] = deps
			g.Order = append(g.Order, name)
		}
		break
	}
	if len(g.Order) == 0 {
		return Graph{}, fmt.Errorf("таблица модулей не найдена: нужны колонки «Модуль» и «Зависит от»")
	}
	for _, m := range g.Order {
		for _, d := range g.Deps[m] {
			if _, ok := g.Deps[d]; !ok {
				return Graph{}, fmt.Errorf("модуль «%s» зависит от неизвестного «%s»", m, d)
			}
		}
	}
	return g, nil
}

// Root — зерно: единственный модуль, от которого никто не зависит.
func (g Graph) Root() (string, error) {
	used := map[string]bool{}
	for _, ds := range g.Deps {
		for _, d := range ds {
			used[d] = true
		}
	}
	var roots []string
	for _, m := range g.Order {
		if !used[m] {
			roots = append(roots, m)
		}
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("зерно должно быть одно, найдено: %s", strings.Join(roots, ", "))
	}
	return roots[0], nil
}

// Chains — все цепочки от зерна вниз до target (без зерна, с target последним). Для зерна цепочек нет.
func (g Graph) Chains(target string) ([][]string, error) {
	if _, ok := g.Deps[target]; !ok {
		return nil, fmt.Errorf("модуля «%s» нет в графе проекта", target)
	}
	root, err := g.Root()
	if err != nil {
		return nil, err
	}
	if target == root {
		return nil, fmt.Errorf("«%s» — зерно: изменения зерна вливаются флагом --seed", target)
	}
	var out [][]string
	var walk func(m string, path []string)
	walk = func(m string, path []string) {
		for _, d := range g.Deps[m] {
			p := append(append([]string(nil), path...), d)
			if d == target {
				out = append(out, p)
				continue
			}
			walk(d, p)
		}
	}
	walk(root, nil)
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i], ",") < strings.Join(out[j], ",") })
	return out, nil
}

// Check проверяет заданную цепочку: первое звено — зависимость зерна, каждое следующее — зависимость предыдущего.
func (g Graph) Check(chain []string) error {
	root, err := g.Root()
	if err != nil {
		return err
	}
	prev := root
	for _, m := range chain {
		ok := false
		for _, d := range g.Deps[prev] {
			if d == m {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("цепочка не по графу: «%s» не зависит от «%s»", prev, m)
		}
		prev = m
	}
	return nil
}

// Load читает реестр из корня репозитория.
func Load(root string) (Graph, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(Path)))
	if err != nil {
		return Graph{}, fmt.Errorf("нет графа проекта %s: его ведёт агент (см. .sky/readme/readme.md)", Path)
	}
	return Parse(string(b))
}

// Resolve выбирает цепочку до модуля: заданную (проверяет по графу) или единственную возможную.
func (g Graph) Resolve(module string, chain []string) ([]string, error) {
	if len(chain) > 0 {
		if chain[len(chain)-1] != module {
			return nil, fmt.Errorf("цепочка должна кончаться модулем «%s»", module)
		}
		return chain, g.Check(chain)
	}
	cs, err := g.Chains(module)
	if err != nil {
		return nil, err
	}
	switch len(cs) {
	case 0:
		return nil, fmt.Errorf("от зерна до «%s» нет пути в графе", module)
	case 1:
		return cs[0], nil
	}
	var b strings.Builder
	for i, c := range cs {
		if i == 8 {
			fmt.Fprintf(&b, "\n  … ещё %d", len(cs)-8)
			break
		}
		fmt.Fprintf(&b, "\n  --chain %s", strings.Join(c, ","))
	}
	return nil, fmt.Errorf("до «%s» несколько цепочек, задай одну:%s", module, b.String())
}
