package ops

import "github.com/DrBopoH/sky-cli/internal/commit"

var commitOp = Operation{
	Name:   "commit",
	Fields: 1,
	Opt:    len(commit.Flags),
	Doc:    "поля: `сообщение`, дальше необязательные флаги: `new` (то же, что `sky commit --new`: в коммит идут и прочие новые файлы дерева, кроме игнорируемых). После записи файлов делает один коммит из путей патча (записанных и удалённых), без флагов остального в дереве не трогает; пути из .gitignore пропускаются. Не больше одной записи на патч; без неё патч применяется без коммита. Сообщение: `feat|fix|refactor|perf|test|docs|style|chore: текст`, после типа до 72 символов, по-английски.",
	Plan:   planCommit,
}

func planCommit(f []string, _ World) (Result, error) {
	if err := commit.CheckMessage(f[0]); err != nil {
		return Result{}, err
	}
	res := Result{Commit: f[0]}
	for _, fl := range f[1:] {
		if err := res.CommitOpts.Set(fl); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}
