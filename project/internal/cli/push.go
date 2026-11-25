package cli

import (
	"fmt"
	"strings"

	"github.com/DrBopoH/sky-cli/internal/sync"
)

// pushShared отправляет общие ветки, изменённые локальной операцией (land, fix), если есть origin и не задан --local.
// Сбой push не отменяет операцию: она локальная и уже сделана, sky называет, чем повторить.
func pushShared(env Env, root string, before map[string]string, local bool) {
	if local || !sync.HasRemote(root) {
		return
	}
	pushed, err := sync.Publish(root, before)
	for _, n := range pushed {
		fmt.Fprintf(env.Out, "  push %s %s\n", sync.Remote, n)
	}
	if err != nil {
		fmt.Fprintf(env.Out, "! %s\n", strings.TrimSpace(err.Error()))
	}
}
