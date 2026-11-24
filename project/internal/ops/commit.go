package ops

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxSubject = 72

var commitMsg = regexp.MustCompile(`^(feat|fix|refactor|perf|test|docs|style|chore): (\S.*)$`)

var commitOp = Operation{
	Name:   "commit",
	Fields: 1,
	Opt:    len(CommitFlags),
	Doc:    "поля: `сообщение`, дальше необязательные флаги: `new` (то же, что `sky commit --new`: в коммит идут и прочие новые файлы дерева, кроме игнорируемых). После записи файлов делает один коммит из путей патча (записанных и удалённых), без флагов остального в дереве не трогает; пути из .gitignore пропускаются. Не больше одной записи на патч; без неё патч применяется без коммита. Сообщение: `feat|fix|refactor|perf|test|docs|style|chore: текст`, после типа до 72 символов, по-английски.",
	Plan:   planCommit,
}

// CommitFlags — флаги commit; одни и те же у операции (поля после сообщения) и у команды (`--флаг`).
var CommitFlags = []string{"new"}

func planCommit(f []string, _ World) (Result, error) {
	if err := CheckCommitMessage(f[0]); err != nil {
		return Result{}, err
	}
	res := Result{Commit: f[0]}
	for _, fl := range f[1:] {
		switch fl {
		case "new":
			res.New = true
		default:
			return Result{}, fmt.Errorf("неизвестный флаг «%s», допустимы: %s", fl, strings.Join(CommitFlags, ", "))
		}
	}
	return res, nil
}

// CheckCommitMessage — правило сообщения коммита, общее для операции commit и команды sky commit.
func CheckCommitMessage(msg string) error {
	m := commitMsg.FindStringSubmatch(msg)
	if m == nil {
		return fmt.Errorf("сообщение «%s»: нужен вид «тип: текст», тип — feat, fix, refactor, perf, test, docs, style или chore", msg)
	}
	if n := utf8.RuneCountInString(m[2]); n > maxSubject {
		return fmt.Errorf("сообщение: после типа %d символов, допустимо до %d", n, maxSubject)
	}
	return nil
}
