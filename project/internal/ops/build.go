package ops

import (
	"archive/zip"
	"fmt"
	"github.com/DrBopoH/sky-cli/internal/guard"
	"io"
	"os"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/apply"
	"github.com/DrBopoH/sky-cli/internal/notation"
)

const (
	opPrefix     = ".sky/operation/"
	maxFileSize  = 64 << 20
	maxTotalSize = 256 << 20
)

// Build читает архив, проверяет его и строит эффекты: сначала операции, потом файлы патча.
func Build(root, zipPath string) (apply.Plan, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return apply.Plan{}, fmt.Errorf("не открыть архив: %w", err)
	}
	defer zr.Close()

	var files []apply.Effect
	opFiles := map[string]string{}
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return apply.Plan{}, fmt.Errorf("%s: символические ссылки в патче запрещены", name)
		}
		if f.UncompressedSize64 > maxFileSize {
			return apply.Plan{}, fmt.Errorf("%s: файл больше %d МБ", name, maxFileSize>>20)
		}
		total += f.UncompressedSize64
		if total > maxTotalSize {
			return apply.Plan{}, fmt.Errorf("патч распаковывается больше чем в %d МБ", maxTotalSize>>20)
		}
		body, err := readEntry(f)
		if err != nil {
			return apply.Plan{}, fmt.Errorf("%s: %w", name, err)
		}
		if strings.HasPrefix(name, opPrefix) {
			op := strings.TrimPrefix(name, opPrefix)
			if op == "" || strings.Contains(op, "/") {
				return apply.Plan{}, fmt.Errorf("%s: операция — это файл прямо в .sky/operation/", name)
			}
			opFiles[op] = string(body)
			continue
		}
		if err := guard.Path(name); err != nil {
			return apply.Plan{}, err
		}
		files = append(files, apply.Effect{Kind: apply.Write, Path: name, Content: body, Exec: f.Mode()&0o111 != 0})
	}

	var plan apply.Plan
	w := world{root: root}
	for _, o := range All() {
		src, ok := opFiles[o.Name]
		if !ok {
			continue
		}
		delete(opFiles, o.Name)
		recs, err := notation.ParseRange(src, o.Fields, o.Fields+o.Opt)
		if err != nil {
			return apply.Plan{}, fmt.Errorf("операция %s: %w", o.Name, err)
		}
		for _, r := range recs {
			res, err := o.Plan(r, w)
			if err != nil {
				return apply.Plan{}, fmt.Errorf("операция %s: %w", o.Name, err)
			}
			if res.Commit != "" {
				if plan.Commit != "" {
					return apply.Plan{}, fmt.Errorf("операция %s: в патче больше одной записи commit", o.Name)
				}
				plan.Commit = res.Commit
				plan.CommitOpts = res.CommitOpts
			}
			plan.Effects = append(plan.Effects, res.Effects...)
			plan.Notes = append(plan.Notes, res.Notes...)
		}
	}
	for name := range opFiles {
		return apply.Plan{}, fmt.Errorf("неизвестная операция «%s»: эта версия sky её не знает", name)
	}
	plan.Effects = append(plan.Effects, files...)

	if err := apply.CheckEffects(root, plan.Effects); err != nil {
		return apply.Plan{}, err
	}
	return plan, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxFileSize {
		return nil, fmt.Errorf("файл больше %d МБ", maxFileSize>>20)
	}
	return b, nil
}
