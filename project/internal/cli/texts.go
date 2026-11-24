package cli

import (
	_ "embed"
	"strings"
)

// Описания команд: длинный текст лежит в text/, код только ссылается.
var (
	//go:embed text/apply-summary.txt
	summaryApplyRaw string
	//go:embed text/commit-summary.txt
	summaryCommitRaw string
	//go:embed text/look-summary.txt
	summaryLookRaw string
	//go:embed text/up-summary.txt
	summaryUpRaw string
	//go:embed text/init-summary.txt
	summaryInitRaw string
	//go:embed text/rollback-summary.txt
	summaryRollbackRaw string
)

var (
	summaryApply    = strings.TrimSpace(summaryApplyRaw)
	summaryCommit   = strings.TrimSpace(summaryCommitRaw)
	summaryLook     = strings.TrimSpace(summaryLookRaw)
	summaryUp       = strings.TrimSpace(summaryUpRaw)
	summaryInit     = strings.TrimSpace(summaryInitRaw)
	summaryRollback = strings.TrimSpace(summaryRollbackRaw)
)
