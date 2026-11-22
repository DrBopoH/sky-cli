package apply

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/ops"
)

const (
	opPrefix     = ".sky/operation/"
	maxFileSize  = 64 << 20
	maxTotalSize = 256 << 20
)

// Plan — всё, что патч собирается сделать; ничего ещё не записано.
type Plan struct {
	Effects []ops.Effect
	Notes   []string
}

// Build читает архив, проверяет его и строит эффекты: сначала операции, потом файлы патча.
func Build(root, zipPath string) (Plan, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return Plan{}, fmt.Errorf("не открыть архив: %w", err)
	}
	defer zr.Close()

	var files []ops.Effect
	opFiles := map[string]string{}
	var total uint64
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return Plan{}, fmt.Errorf("%s: символические ссылки в патче запрещены", name)
		}
		if f.UncompressedSize64 > maxFileSize {
			return Plan{}, fmt.Errorf("%s: файл больше %d МБ", name, maxFileSize>>20)
		}
		total += f.UncompressedSize64
		if total > maxTotalSize {
			return Plan{}, fmt.Errorf("патч распаковывается больше чем в %d МБ", maxTotalSize>>20)
		}
		body, err := readEntry(f)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", name, err)
		}
		if strings.HasPrefix(name, opPrefix) {
			op := strings.TrimPrefix(name, opPrefix)
			if op == "" || strings.Contains(op, "/") {
				return Plan{}, fmt.Errorf("%s: операция — это файл прямо в .sky/operation/", name)
			}
			opFiles[op] = string(body)
			continue
		}
		if err := GuardPath(name); err != nil {
			return Plan{}, err
		}
		files = append(files, ops.Effect{Kind: ops.Write, Path: name, Content: body, Exec: f.Mode()&0o111 != 0})
	}

	var plan Plan
	w := world{root: root}
	for _, o := range ops.All() {
		src, ok := opFiles[o.Name]
		if !ok {
			continue
		}
		delete(opFiles, o.Name)
		recs, err := ops.Parse(src, o.Fields)
		if err != nil {
			return Plan{}, fmt.Errorf("операция %s: %w", o.Name, err)
		}
		for _, r := range recs {
			res, err := o.Plan(r, w)
			if err != nil {
				return Plan{}, fmt.Errorf("операция %s: %w", o.Name, err)
			}
			plan.Effects = append(plan.Effects, res.Effects...)
			plan.Notes = append(plan.Notes, res.Notes...)
		}
	}
	for name := range opFiles {
		return Plan{}, fmt.Errorf("неизвестная операция «%s»: эта версия sky её не знает", name)
	}
	plan.Effects = append(plan.Effects, files...)

	if err := checkEffects(root, plan.Effects); err != nil {
		return Plan{}, err
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

// checkEffects: единая проверка путей всех эффектов, конфликты «путь дважды» и «файл и папка», выход за корень.
func checkEffects(root string, effects []ops.Effect) error {
	seen := map[string]bool{}
	dirs := map[string]bool{}
	for _, e := range effects {
		if err := GuardPath(e.Path); err != nil {
			return err
		}
		if seen[e.Path] {
			return fmt.Errorf("%s: путь встречается в патче больше одного раза", e.Path)
		}
		seen[e.Path] = true
		for i := 0; i < len(e.Path); i++ {
			if e.Path[i] == '/' {
				dirs[e.Path[:i]] = true
			}
		}
	}
	for p := range seen {
		if dirs[p] {
			return fmt.Errorf("%s: в патче и файл, и папка с таким путём", p)
		}
	}
	for _, e := range effects {
		if _, err := resolve(root, e.Path); err != nil {
			return err
		}
	}
	return nil
}
