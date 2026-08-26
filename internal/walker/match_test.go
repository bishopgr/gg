package walker

import "testing"

func TestMatchOneCaseAware(t *testing.T) {
	tests := []struct {
		name            string
		pattern, target string
		caseInsensitive bool
		want            bool
	}{
		{"exact match, case sensitive", "error.log", "error.log", false, true},
		{"different case, case sensitive: no match", "ERROR.LOG", "error.log", false, false},
		{"different case, case insensitive: match", "ERROR.LOG", "error.log", true, true},
		{"glob star, case sensitive", "*.log", "error.log", false, true},
		{"glob star, case insensitive, mixed case pattern", "*.LOG", "error.log", true, true},
		{"glob star, case sensitive, mismatched case: no match", "*.LOG", "error.log", false, false},
		{"no match regardless of case", "*.txt", "error.log", true, false},
		{"question mark glob", "err?r.log", "error.log", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchOneCaseAware(tt.pattern, tt.target, tt.caseInsensitive)
			if got != tt.want {
				t.Errorf("matchOneCaseAware(%q, %q, %v) = %v, want %v",
					tt.pattern, tt.target, tt.caseInsensitive, got, tt.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"*.log", "*.tmp"}
	if !matchAny(patterns, "debug.log") {
		t.Error("expected debug.log to match *.log")
	}
	if matchAny(patterns, "main.go") {
		t.Error("did not expect main.go to match any pattern")
	}
	if matchAny(nil, "anything") {
		t.Error("empty pattern list should match nothing")
	}
}
