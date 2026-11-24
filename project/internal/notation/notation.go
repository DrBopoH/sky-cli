// Package notation — общая запись данных sky: одна запись на строку, поля через табуляцию.
// Так записаны файлы операций в патче и manifest.txt записей history.
package notation

import (
	"fmt"
	"strings"
)

// Parse — общая нотация: одна запись на строку, поля через табуляцию, пустые строки и «#…» пропускаются.
func Parse(src string, fields int) ([][]string, error) { return ParseRange(src, fields, fields) }

// ParseRange — то же, но полей от min до max (хвост необязательный).
func ParseRange(src string, min, max int) ([][]string, error) {
	var out [][]string
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < min || len(parts) > max {
			if min == max {
				return nil, fmt.Errorf("строка %d: нужно полей %d, найдено %d", i+1, min, len(parts))
			}
			return nil, fmt.Errorf("строка %d: нужно полей от %d до %d, найдено %d", i+1, min, max, len(parts))
		}
		for _, p := range parts {
			if p == "" {
				return nil, fmt.Errorf("строка %d: пустое поле", i+1)
			}
		}
		out = append(out, parts)
	}
	return out, nil
}
