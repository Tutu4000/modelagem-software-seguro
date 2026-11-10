package support

import (
	"strings"
	"testing"
)

func TestCleanText(t *testing.T) {
	if got, err := CleanText("  linha 1\r\nlinha 2 <b>ok</b> ", 100); err != nil || got != "linha 1\nlinha 2 <b>ok</b>" {
		t.Errorf("CleanText = %q, %v", got, err)
	}
	for name, in := range map[string]string{
		"empty": "  ", "bell": "a\u0007b", "escape": "a\x1b[31mb", "too long": strings.Repeat("a", 101), "invalid UTF-8": "\xff",
	} {
		if _, err := CleanText(in, 100); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
