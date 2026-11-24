package apply

import "fmt"

// CheckEffects: единая проверка путей всех эффектов, конфликты «путь дважды» и «файл и папка», выход за корень.
func CheckEffects(root string, effects []Effect) error {
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
