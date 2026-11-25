// Package named — версия именованного коммита: сообщение «резюме | версия» по регламенту (3.5).
// На ветках модулей версия — `vC.hash`, в dev — `vR.B.C+T.hash`.
package named

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Version — версия именованного коммита. Full: R, B, C, T заданы (dev); иначе задан только C (ветка модуля).
// Сборка: State — буква состояния (a, b, s, m), B — номер сборки, F — номер исправления (0 — сама сборка).
type Version struct {
	Full       bool
	R, B, C, T int
	State      byte
	F          int
	Hash       string
}

// IsBuild сообщает, что это версия сборки (v1.a1.29+29, v1.a1-2.29+31).
func (v Version) IsBuild() bool { return v.State != 0 }

// Grep — расширенное регулярное выражение для git log --grep -E: сообщение оканчивается версией именованного коммита.
const Grep = ` [|] v([0-9]+[.][0-9]+[.][0-9]+[+][0-9]+|[0-9]+)[.][0-9a-f]{7}$`

var msgRe = regexp.MustCompile(`^(.*) \| v(?:(\d+)\.(\d+)\.(\d+)\+(\d+)|(\d+))\.([0-9a-f]{7})$`)

var buildRe = regexp.MustCompile(`^(.*) \| v(\d+)\.([absm])(\d+)(?:-(\d+))?\.(\d+)\+(\d+)\.([0-9a-f]{7})$`)

// Parse разбирает первую строку сообщения коммита; ok=false, если коммит не именованный.
func Parse(subject string) (summary string, v Version, ok bool) {
	if b := buildRe.FindStringSubmatch(subject); b != nil {
		v = Version{Full: true, State: b[3][0], Hash: b[8]}
		v.R, _ = strconv.Atoi(b[2])
		v.B, _ = strconv.Atoi(b[4])
		v.F, _ = strconv.Atoi(b[5])
		v.C, _ = strconv.Atoi(b[6])
		v.T, _ = strconv.Atoi(b[7])
		return b[1], v, true
	}
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
	if v.IsBuild() {
		f := ""
		if v.F > 0 {
			f = fmt.Sprintf("-%d", v.F)
		}
		return fmt.Sprintf("v%d.%c%d%s.%d+%d.%s", v.R, v.State, v.B, f, v.C, v.T, v.Hash)
	}
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

// MaxLine — наибольшая длина резюме и названия в рунах (после типа в сообщении коммита — тот же предел).
const MaxLine = 72

// CheckLine проверяет резюме или название: что оно есть, одной строкой и не длиннее MaxLine.
// what — как называть проверяемое в ошибке («резюме», «название»).
func CheckLine(what, s string) error {
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("нужно %s", what)
	case strings.ContainsAny(s, "\r\n"):
		return fmt.Errorf("%s одной строкой", what)
	case utf8.RuneCountInString(s) > MaxLine:
		return fmt.Errorf("%s длиннее %d символов", what, MaxLine)
	}
	return nil
}
