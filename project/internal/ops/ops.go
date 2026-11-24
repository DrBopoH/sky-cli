// Package ops — реестр операций патча: декларативные инструкции из .sky/operation/ внутри архива.
// Операция превращает запись в эффекты; всё остальное (снимок, откат, защита путей) знает только эффекты.
package ops

import (
	"fmt"
	"strings"
)

type Kind int

const (
	Write Kind = iota
	Delete
)

type Effect struct {
	Kind    Kind
	Path    string // от корня репозитория, с «/»
	Content []byte // для Write
	Exec    bool   // для Write
}

// World — то, что операция может узнать о репозитории.
type World interface {
	Guard(path string) error
	Stat(path string) (exists, isDir bool)
}

type Result struct {
	Effects []Effect
	Notes   []string
	Commit  string // сообщение коммита из путей патча; делается после записи файлов
	New     bool   // флаг new: в коммит идут и прочие новые файлы дерева (как sky commit --new)
}

type Operation struct {
	Name   string
	Fields int    // число обязательных полей записи
	Opt    int    // сколько ещё необязательных полей (флаги) может идти следом
	Doc    string // продолжение строки «- `имя` — …» в readme.md
	Plan   func(fields []string, w World) (Result, error)
}

var registry = []Operation{deleteOp, commitOp}

func All() []Operation { return append([]Operation(nil), registry...) }

// Parse — общая нотация: одна запись на строку, поля через табуляцию, пустые строки и «#…» пропускаются.
func Parse(src string, fields int) ([][]string, error) { return ParseRange(src, fields, fields) }

// ParseRange — то же, но полей от min до max (хвост необязательный).
func ParseRange(src string, min, max int) ([][]string, error) {
	var out [][]string
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < min || len(parts) > max {
			if min == max {
				return nil, fmt.Errorf("строка %d: нужно полей %d, найдено %d", i+1, min, len(parts))
			}
			return nil, fmt.Errorf("строка %d: нужно полей от %d до %d, найдено %d", i+1, min, max, len(parts))
		}
		for _, p := range parts {
			if p == "" {
				return nil, fmt.Errorf("строка %d: пустое поле", i+1)
			}
		}
		out = append(out, parts)
	}
	return out, nil
}
