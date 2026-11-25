package graph

import (
	"strings"
	"testing"
)

const doc = "# G\n\n## 2. Модули\n\n| Модуль | Ответственность | Зависит от |\n|---|---|---|\n| leaf | лист | — |\n| `mid` | середина | leaf |\n| `side` | сбоку | leaf |\n| top | верх | mid, side |\n| main | зерно | top |\n\nтекст\n"

func TestParseRootChains(t *testing.T) {
	g, err := Parse(doc)
	if err != nil || len(g.Order) != 5 {
		t.Fatalf("%v %v", g.Order, err)
	}
	if r, err := g.Root(); err != nil || r != "main" {
		t.Fatalf("root %q %v", r, err)
	}
	cs, err := g.Chains("leaf")
	if err != nil || len(cs) != 2 || strings.Join(cs[0], ">") != "top>mid>leaf" || strings.Join(cs[1], ">") != "top>side>leaf" {
		t.Fatalf("%v %v", cs, err)
	}
	if cs, _ := g.Chains("top"); len(cs) != 1 || strings.Join(cs[0], ">") != "top" {
		t.Fatalf("%v", cs)
	}
	if _, err := g.Chains("main"); err == nil {
		t.Fatal("зерно без цепочки")
	}
	if _, err := g.Chains("nope"); err == nil {
		t.Fatal("нет модуля")
	}
}

func TestCheckChain(t *testing.T) {
	g, _ := Parse(doc)
	if err := g.Check([]string{"top", "mid", "leaf"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Check([]string{"top", "leaf"}); err == nil {
		t.Fatal("top не зависит от leaf напрямую")
	}
	if err := g.Check([]string{"mid"}); err == nil {
		t.Fatal("зерно не зависит от mid")
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse("нет таблицы"); err == nil {
		t.Fatal("нет таблицы")
	}
	if _, err := Parse("| Модуль | Зависит от |\n|---|---|\n| a | b |\n"); err == nil {
		t.Fatal("неизвестная зависимость")
	}
}
