// Package version — версия sky. В сборке по регламенту её подставляет sky build через -ldflags
// (переменная SKY_BUILD_VERSION); при обычной сборке (go build) версия — «dev» и короткий хэш коммита, если он известен.
package version

import "runtime/debug"

// Version подставляется через -ldflags "-X github.com/DrBopoH/sky-cli/internal/version.Version=…".
var Version = "dev"

// String — версия для печати: подставленная при сборке, иначе «dev» с хэшем коммита из сведений о сборке Go.
func String() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	rev, dirty := "", false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				rev = s.Value[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if dirty {
		return "dev+" + rev + "-dirty"
	}
	return "dev+" + rev
}
