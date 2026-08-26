package walker

import "testing"

// FuzzMatchOneCaseAware and FuzzExpandGlobs guard against panics from
// adversarial glob patterns/names — filepath.Match is documented to
// return ErrBadPattern rather than panic, but pattern parsing is exactly
// the class of thing PROCESS.md §5 calls out for fuzzing rather than
// trusting the doc comment alone.

func FuzzMatchOneCaseAware(f *testing.F) {
	seeds := []struct {
		pattern, name   string
		caseInsensitive bool
	}{
		{"*.go", "main.go", false},
		{"[", "x", false},
		{"[unclosed", "anything", true},
		{"**", "", false},
		{"a\\", "a", false},
	}
	for _, s := range seeds {
		f.Add(s.pattern, s.name, s.caseInsensitive)
	}

	f.Fuzz(func(t *testing.T, pattern, name string, caseInsensitive bool) {
		_ = matchOneCaseAware(pattern, name, caseInsensitive) // must not panic
	})
}

func FuzzExpandGlobs(f *testing.F) {
	seeds := []string{"", "*.go", "[unclosed", "a[1].txt", "**", "plain", "..\\..\\x"}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, arg string) {
		_, _ = ExpandGlobs([]string{arg}) // must not panic; error is a valid outcome
	})
}
