package look

import (
	_ "embed"
	"strings"
)

// Первая строка каждого файла foryou/: что внутри и когда читать. Текст лежит в text/; %d подставляет код.
var (
	//go:embed text/state.txt
	preStateRaw string
	//go:embed text/branches.txt
	preBranchesRaw string
	//go:embed text/history.txt
	preHistoryRaw string
	//go:embed text/files.txt
	preFilesRaw string
	//go:embed text/tree.txt
	preTreeRaw string
	//go:embed text/stages.txt
	preStagesRaw string
)

var (
	preState    = strings.TrimSpace(preStateRaw)
	preBranches = strings.TrimSpace(preBranchesRaw)
	preHistory  = strings.TrimSpace(preHistoryRaw)
	preFiles    = strings.TrimSpace(preFilesRaw)
	preTree     = strings.TrimSpace(preTreeRaw)
	preStages   = strings.TrimSpace(preStagesRaw)
)
