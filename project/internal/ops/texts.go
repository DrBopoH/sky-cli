package ops

import (
	_ "embed"
	"strings"
)

// Описания операций для readme.md: текст лежит в text/.
var (
	//go:embed text/commit.txt
	docCommitRaw string
	//go:embed text/delete.txt
	docDeleteRaw string

	docCommit = strings.TrimSpace(docCommitRaw)
	docDelete = strings.TrimSpace(docDeleteRaw)
)
