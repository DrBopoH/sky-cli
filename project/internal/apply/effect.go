package apply

import "github.com/DrBopoH/sky-cli/internal/commit"

// Kind — что эффект делает с файлом.
type Kind int

const (
	Write Kind = iota
	Delete
)

// Effect — одно действие над файлом проекта. Всё, что пишет, откатывает и снимает apply, знает только эффекты.
type Effect struct {
	Kind    Kind
	Path    string // от корня репозитория, с «/»
	Content []byte // для Write
	Exec    bool   // для Write
}

// Plan — всё, что патч собирается сделать; ничего ещё не записано.
type Plan struct {
	Effects    []Effect
	Notes      []string
	Commit     string         // сообщение коммита; пусто — без коммита
	CommitOpts commit.Options // параметры использования коммита
}

// Planner строит план из архива патча. Реализует адаптер патча (ops.Build); ядро его не знает и получает при сборке.
type Planner func(root, patchPath string) (Plan, error)
