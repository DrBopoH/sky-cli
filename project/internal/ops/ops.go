// Package ops — реестр операций патча: декларативные инструкции из .sky/operation/ внутри архива.
// Операция превращает запись в эффекты; всё остальное (снимок, откат, защита путей) знает только эффекты.
package ops

import (
	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/commit"
)

// World — то, что операция может узнать о репозитории.
type World interface {
	Guard(path string) error
	Stat(path string) (exists, isDir bool)
}

type Result struct {
	Effects    []apply.Effect
	Notes      []string
	Commit     string         // сообщение коммита из путей патча; делается после записи файлов
	CommitOpts commit.Options // параметры использования коммита (флаги записи)
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
