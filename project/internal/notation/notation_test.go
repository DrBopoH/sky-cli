package notation

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse("# c\n\na b/c.txt\r\n\nx\n", 1)
	if err != nil || !reflect.DeepEqual(got, [][]string{{"a b/c.txt"}, {"x"}}) {
		t.Fatalf("%v %v", got, err)
	}
	got, err = Parse("a\tb\n", 2)
	if err != nil || !reflect.DeepEqual(got, [][]string{{"a", "b"}}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"a\tb\n", "\ta\n"} {
		if _, err := Parse(bad, 1); err == nil {
			t.Errorf("%q: ждали ошибку", bad)
		}
	}
	if _, err := Parse("a\n", 2); err == nil || !strings.Contains(err.Error(), "строка 1") {
		t.Errorf("номер строки в ошибке: %v", err)
	}
}
