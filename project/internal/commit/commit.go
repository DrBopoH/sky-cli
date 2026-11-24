// Package commit — правила коммита: сообщение и параметры использования. Общие для всех интерфейсов (консоль, операции патча):
// каждый адаптер переводит свою запись в эти правила и параметры, а не хранит собственных.
package commit

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxSubject = 72

var message = regexp.MustCompile(`^(feat|fix|refactor|perf|test|docs|style|chore): (\S.*)$`)

// CheckMessage — правило сообщения: «тип: текст», после типа не больше 72 символов.
func CheckMessage(msg string) error {
	m := message.FindStringSubmatch(msg)
	if m == nil {
		return fmt.Errorf("сообщение «%s»: нужен вид «тип: текст», тип — feat, fix, refactor, perf, test, docs, style или chore", msg)
	}
	if n := utf8.RuneCountInString(m[2]); n > maxSubject {
		return fmt.Errorf("сообщение: после типа %d символов, допустимо до %d", n, maxSubject)
	}
	return nil
}

// Flags — имена параметров использования; адаптеры сами решают, как их записывать (`--new`, поле `new`).
var Flags = []string{"new"}

// Options — параметры использования коммита.
type Options struct {
	New bool // в коммит идут и прочие новые файлы дерева, кроме игнорируемых
}

// Set включает параметр по имени.
func (o *Options) Set(flag string) error {
	switch flag {
	case "new":
		o.New = true
		return nil
	}
	return fmt.Errorf("неизвестный флаг «%s», допустимы: %s", flag, strings.Join(Flags, ", "))
}
