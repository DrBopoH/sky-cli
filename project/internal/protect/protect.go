// Package protect — удалённая часть, этап R4 (регламент, 3.1.3): защита общих веток на GitHub правилами (rulesets).
// Это голова над ghx: запрет удалять main, dev и ветки сборок и запрет force-push в main, dev, ветки сборок и модулей
// (двухсегментные имена «x/ММДД-ЧЧММ»; трёхсегментные wip-ветки автора не затрагиваются). Правила — данные в text/*.json;
// команда создаёт их или обновляет по имени, повторный запуск ничего не дублирует.
package protect

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/DrBopoH/sky-cli/internal/ghx"
)

var (
	//go:embed text/no-deletion.json
	noDeletion string
	//go:embed text/no-force-push.json
	noForcePush string
)

// Options — параметры одного вызова.
type Options struct {
	Root string
	Dry  bool
}

// Result — что сделано (или что было бы сделано при Dry).
type Result struct {
	Lines []string
}

type ruleset struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Run применяет правила: недостающие создаёт, существующие (по имени) обновляет.
func Run(o Options) (Result, error) {
	res := Result{}
	say := func(f string, a ...any) { res.Lines = append(res.Lines, fmt.Sprintf(f, a...)) }
	if !ghx.Available() {
		return res, fmt.Errorf("не найдена команда gh (GitHub CLI): установи её и войди (gh auth login)")
	}
	out, err := ghx.Run(o.Root, "api", "repos/:owner/:repo/rulesets")
	if err != nil {
		return res, fmt.Errorf("правила репозитория не прочитаны (нужны права администратора): %v", err)
	}
	var have []ruleset
	if err := json.Unmarshal([]byte(out), &have); err != nil {
		return res, fmt.Errorf("ответ gh не разобран: %v", err)
	}
	for _, body := range []string{noDeletion, noForcePush} {
		var r ruleset
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			return res, fmt.Errorf("встроенное правило испорчено: %v", err)
		}
		id := 0
		for _, h := range have {
			if h.Name == r.Name {
				id = h.ID
			}
		}
		verb, args := "создано", []string{"api", "-X", "POST", "repos/:owner/:repo/rulesets", "--input", "-"}
		if id != 0 {
			verb, args = "обновлено", []string{"api", "-X", "PUT", fmt.Sprintf("repos/:owner/:repo/rulesets/%d", id), "--input", "-"}
		}
		if o.Dry {
			say("  %s: будет %s", r.Name, verb)
			continue
		}
		if _, err := ghx.RunInput(o.Root, body, args...); err != nil {
			return res, fmt.Errorf("правило «%s» не применено: %v", r.Name, err)
		}
		say("  %s: %s", r.Name, verb)
	}
	return res, nil
}
