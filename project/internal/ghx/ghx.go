// Package ghx — единственная точка доступа к GitHub CLI (gh), как gitx для git.
package ghx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Available сообщает, найдена ли команда gh.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Run запускает gh в папке root и возвращает стандартный вывод; при ошибке в тексте — вывод ошибок gh.
func Run(root string, args ...string) (string, error) { return RunInput(root, "", args...) }

// RunInput — то же, но input подаётся на стандартный ввод (для gh api --input -).
func RunInput(root, input string, args ...string) (string, error) {
	c := exec.Command("gh", args...)
	c.Dir = root
	if input != "" {
		c.Stdin = strings.NewReader(input)
	}
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		name := strings.Join(args[:min(2, len(args))], " ")
		return out.String(), fmt.Errorf("gh %s: %v: %s", name, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
