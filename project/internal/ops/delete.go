package ops

import "fmt"

var deleteOp = Operation{
	Name:   "delete",
	Fields: 1,
	Doc:    "поле: `путь`. Удаляет файл. Папки и маски не поддерживаются.",
	Plan:   planDelete,
}

func planDelete(f []string, w World) (Result, error) {
	p := f[0]
	if err := w.Guard(p); err != nil {
		return Result{}, err
	}
	exists, isDir := w.Stat(p)
	switch {
	case !exists:
		return Result{Notes: []string{p + ": файла уже нет, пропущено"}}, nil
	case isDir:
		return Result{}, fmt.Errorf("%s: это папка, delete удаляет только файлы", p)
	}
	return Result{Effects: []Effect{{Kind: Delete, Path: p}}}, nil
}
