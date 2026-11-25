// Package vhist читает версии из истории git: последний именованный коммит dev, сборки и их влияние на следующую версию.
// Общий источник для land и build: оба считают версию одинаково.
package vhist

import (
	"strconv"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/gitx"
	"github.com/DrBopoH/sky-cli/internal/named"
)

// H — чтение истории репозитория Root; From — «R.B.C+T», точка отсчёта, пока именованных коммитов нет.
type H struct {
	Root string
	From string
}

// Base — точка отсчёта.
func (h H) Base() (named.Version, error) {
	if h.From == "" {
		return named.Version{Full: true, R: 1, B: 1}, nil
	}
	return named.ParseBase(h.From)
}

// Last — версия последнего именованного коммита первой линии rev; нет такого — точка отсчёта (found=false).
func (h H) Last(rev string) (v named.Version, found bool, err error) {
	s := strings.TrimSpace(gitx.Try(h.Root, "log", "-1", "--first-parent", "-E", "--grep="+named.Grep, "--format=%s", rev))
	if s != "" {
		if _, v, ok := named.Parse(s); ok && v.Full && !v.IsBuild() {
			return v, true, nil
		}
	}
	b, err := h.Base()
	return b, false, err
}

// Build — хвост ветки сборки: сама сборка, не исправление.
type Build struct {
	Hash    string // полный хэш хвоста
	Version named.Version
	Source  string // полный хэш коммита dev, с которого снята сборка
}

// commits — все коммиты сборок (хвосты и исправления) во всех ветках «build/…».
func (h H) commits() []Build {
	out := gitx.Try(h.Root, "log", "--branches=build/*", "--format=%H%x09%s")
	seen := map[string]bool{}
	var bs []Build
	for _, ln := range strings.Split(out, "\n") {
		hash, subj, ok := strings.Cut(ln, "\t")
		if !ok || seen[hash] {
			continue
		}
		seen[hash] = true
		_, v, ok := named.Parse(subj)
		if !ok || !v.IsBuild() {
			continue
		}
		b := Build{Hash: hash, Version: v}
		if v.F == 0 {
			b.Source = strings.TrimSpace(gitx.Try(h.Root, "rev-parse", "--verify", "-q", v.Hash+"^{commit}"))
		}
		bs = append(bs, b)
	}
	return bs
}

// Builds — хвосты сборок во всех ветках «build/…».
func (h H) Builds() []Build {
	var bs []Build
	for _, b := range h.commits() {
		if b.Version.F == 0 {
			bs = append(bs, b)
		}
	}
	return bs
}

// MaxT — наибольшее T среди сборок и исправлений с этим R (0, если их нет).
func (h H) MaxT(r int) int {
	m := 0
	for _, b := range h.commits() {
		if b.Version.R == r && b.Version.T > m {
			m = b.Version.T
		}
	}
	return m
}

// Latest — ветка и версия головы самой поздней сборки (по времени хвоста).
func (h H) Latest() (branch string, head named.Version, ok bool) {
	var best Build
	var bt int
	for _, b := range h.Builds() {
		t, _ := strconv.Atoi(strings.TrimSpace(gitx.Try(h.Root, "log", "-1", "--format=%ct", b.Hash)))
		if !ok || t > bt {
			best, bt, ok = b, t, true
		}
	}
	if !ok {
		return "", named.Version{}, false
	}
	branch = h.Branch(best)
	if branch == "" {
		return "", named.Version{}, false
	}
	head, ok = h.Head(branch)
	return branch, head, ok
}

// Head — версия головы ветки сборки (хвост или последнее исправление).
func (h H) Head(branch string) (named.Version, bool) {
	s := strings.TrimSpace(gitx.Try(h.Root, "log", "-1", "--format=%s", "refs/heads/"+branch))
	_, v, ok := named.Parse(s)
	if !ok || !v.IsBuild() {
		return named.Version{}, false
	}
	return v, true
}

// BuildOf — сборка с этими R и B, если есть.
func (h H) BuildOf(r, b int) (Build, bool) {
	for _, x := range h.Builds() {
		if x.Version.R == r && x.Version.B == b {
			return x, true
		}
	}
	return Build{}, false
}

// Branch — ветка «build/…», на которой лежит хвост сборки.
func (h H) Branch(b Build) string {
	out := gitx.Try(h.Root, "for-each-ref", "--contains", b.Hash, "--format=%(refname:short)", "refs/heads/")
	for _, n := range strings.Split(out, "\n") {
		if strings.HasPrefix(n, "build/") {
			return n
		}
	}
	return ""
}

// Effective — версия, от которой считается следующее изменение: Last, но если после сборки с этим R.B
// прошло ровно то, с чего её сняли (источник сборки — предок rev или сам rev), то B+1 и C=0
// (регламент, 3.6.10: первое именованное изменение после сборки — B+1, C=1). T продолжается.
func (h H) Effective(rev string) (named.Version, bool, error) {
	v, found, err := h.Last(rev)
	if err != nil || !found {
		return v, found, err
	}
	if b, ok := h.BuildOf(v.R, v.B); ok && b.Source != "" && gitx.OK(h.Root, "merge-base", "--is-ancestor", b.Source, rev) {
		v.B++
		v.C = 0
	}
	if m := h.MaxT(v.R); m > v.T {
		v.T = m // исправления сборок тоже растят T (регламент, 3.5.9)
	}
	return v, found, nil
}
