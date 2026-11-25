// Package named — версия именованного коммита: сообщение «резюме | версия» по регламенту (3.5).
// На ветках модулей версия — `vC.hash`, в dev — `vR.B.C+T.hash`.
package named

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version — версия именованного коммита. Full: R, B, C, T заданы (dev); иначе задан только C (ветка модуля).
type Version struct {
	Full       bool
	R, B, C, T int
	Hash       string
}

// Grep — расширенное регулярное выражение для git log --grep -E: сообщение оканчивается версией именованного коммита.
const Grep = ` [|] v([0-9]+[.][0-9]+[.][0-9]+[+][0-9]+|[0-9]+)[.][0-9a-f]{7}$`

var msgRe = regexp.MustCompile(`^(.*) \| v(?:(\d+)\.(\d+)\.(\d+)\+(\d+)|(\d+))\.([0-9a-f]{7})$`)

// Parse разбирает первую строку сообщения коммита; ok=false, если коммит не именованный.
func Parse(subject string) (summary string, v Version, ok bool) {
	m := msgRe.FindStringSubmatch(subject)
	if m == nil {
		return "", Version{}, false
	}
	v.Hash = m[7]
	if m[6] != "" {
		v.C, _ = strconv.Atoi(m[6])
		return m[1], v, true
	}
	v.Full = true
	v.R, _ = strconv.Atoi(m[2])
	v.B, _ = strconv.Atoi(m[3])
	v.C, _ = strconv.Atoi(m[4])
	v.T, _ = strconv.Atoi(m[5])
	return m[1], v, true
}

// String — версия в записи сообщения: v1.1.29+29.def5678 или v28.abc1234.
func (v Version) String() string {
	if v.Full {
		return fmt.Sprintf("v%d.%d.%d+%d.%s", v.R, v.B, v.C, v.T, v.Hash)
	}
	return fmt.Sprintf("v%d.%s", v.C, v.Hash)
}

// Message — сообщение именованного коммита: «резюме | версия».
func Message(summary string, v Version) string { return summary + " | " + v.String() }

var baseRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\+(\d+)$`)

// ParseBase разбирает «R.B.C+T» без хэша: исходную точку отсчёта (например, 1.1.35+35).
func ParseBase(s string) (Version, error) {
	m := baseRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("версия «%s»: нужен вид R.B.C+T, например 1.1.35+35", s)
	}
	v := Version{Full: true}
	v.R, _ = strconv.Atoi(m[1])
	v.B, _ = strconv.Atoi(m[2])
	v.C, _ = strconv.Atoi(m[3])
	v.T, _ = strconv.Atoi(m[4])
	return v, nil
}
