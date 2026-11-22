package ops

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

const maxSubject = 72

var commitMsg = regexp.MustCompile(`^(feat|fix|refactor|perf|test|docs|style|chore): (\S.*)$`)

var commitOp = Operation{
	Name:   "commit",
	Fields: 1,
	Doc:    "поле: `сообщение`. После записи файлов делает один коммит из путей патча (записанных и удалённых), остального в дереве не трогает; пути из .gitignore пропускаются. Не больше одной записи на патч; без неё патч применяется без коммита. Сообщение: `feat|fix|refactor|perf|test|docs|style|chore: текст`, после типа до 72 символов, по-английски.",
	Plan:   planCommit,
}

func planCommit(f []string, _ World) (Result, error) {
	if err := CheckCommitMessage(f[0]); err != nil {
		return Result{}, err
	}
	return Result{Commit: f[0]}, nil
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
