package grep

import (
	"bytes"
	"os"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		in      string
		want    ColorMode
		wantErr bool
	}{
		{"", ColorAuto, false},
		{"auto", ColorAuto, false},
		{"AUTO", ColorAuto, false},
		{"always", ColorAlways, false},
		{"never", ColorNever, false},
		{"sometimes", ColorAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseColorMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseColorMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseColorMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveColor_AlwaysAndNeverIgnoreWriterType(t *testing.T) {
	var buf bytes.Buffer // not a *os.File, so ColorAuto could never be true for it

	if !resolveColor(ColorAlways, &buf) {
		t.Error("resolveColor(ColorAlways, non-file) = false, want true")
	}
	if resolveColor(ColorNever, &buf) {
		t.Error("resolveColor(ColorNever, non-file) = true, want false")
	}
}

func TestResolveColor_AutoIsFalseForNonFileWriter(t *testing.T) {
	var buf bytes.Buffer
	if resolveColor(ColorAuto, &buf) {
		t.Error("resolveColor(ColorAuto, non-file) = true, want false")
	}
}

func TestResolveColor_AutoIsFalseForRedirectedFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if resolveColor(ColorAuto, f) {
		t.Error("resolveColor(ColorAuto, regular file) = true, want false")
	}
}
