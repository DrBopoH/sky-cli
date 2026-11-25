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
	//go:embed text/land-summary.txt
	summaryLandRaw string
	//go:embed text/build-summary.txt
	summaryBuildRaw string
	//go:embed text/fix-summary.txt
	summaryFixRaw string
	//go:embed text/feature-summary.txt
	summaryFeatureRaw string
	//go:embed text/modules-summary.txt
	summaryModulesRaw string
	//go:embed text/publish-summary.txt
	summaryPublishRaw string
	//go:embed text/pr-summary.txt
	summaryPrRaw string
	//go:embed text/sync-summary.txt
	summarySyncRaw string
)

var (
	summaryApply    = strings.TrimSpace(summaryApplyRaw)
	summaryCommit   = strings.TrimSpace(summaryCommitRaw)
	summaryLook     = strings.TrimSpace(summaryLookRaw)
	summaryUp       = strings.TrimSpace(summaryUpRaw)
	summaryInit     = strings.TrimSpace(summaryInitRaw)
	summaryRollback = strings.TrimSpace(summaryRollbackRaw)
	summaryLand     = strings.TrimSpace(summaryLandRaw)
	summaryBuild    = strings.TrimSpace(summaryBuildRaw)
	summaryFix      = strings.TrimSpace(summaryFixRaw)
	summaryFeature  = strings.TrimSpace(summaryFeatureRaw)
	summaryModules  = strings.TrimSpace(summaryModulesRaw)
	summaryPublish  = strings.TrimSpace(summaryPublishRaw)
	summaryPr       = strings.TrimSpace(summaryPrRaw)
	summarySync     = strings.TrimSpace(summarySyncRaw)
)
