package apply

import (
	"github.com/DrBopoH/sky-cli/internal/gitx"
)

// commitPaths делает один коммит из перечисленных путей (записанных и удалённых). Пути из .gitignore пропускаются.
// Возвращает короткий хэш; пустой, если коммитить нечего.
func commitPaths(root, msg string, paths []string) (hash string, skipped []string, err error) {
	var take []string
	for _, p := range paths {
		ig, err := gitx.Ignored(root, p)
		if err != nil {
			return "", nil, err
		}
		if ig {
			skipped = append(skipped, p)
			continue
		}
		take = append(take, p)
	}
	if len(take) == 0 {
		return "", skipped, nil
	}
	if _, err := gitx.Run(root, append([]string{"add", "-A", "--"}, take...)...); err != nil {
		return "", skipped, err
	}
	if _, err := gitx.Run(root, append([]string{"commit", "-q", "-m", msg, "--"}, take...)...); err != nil {
		return "", skipped, err
	}
	hash, err = gitx.Run(root, "rev-parse", "--short=7", "HEAD")
	return hash, skipped, err
}
