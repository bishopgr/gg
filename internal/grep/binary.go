package grep

// BinaryMode controls how a file internal/encoding.Sniff detects as
// binary is handled during matching, mirroring GNU grep's
// -a/--text/-I trio (a narrower slice of --binary-files=TYPE — see
// LIMITATIONS in README.md for what's deliberately not covered).
type BinaryMode int

const (
	// BinaryDefault prints GNU grep's conventional
	// "Binary file FILENAME matches" message in place of raw line
	// content when a binary-detected file matches — the behavior before
	// -a/-I existed, unchanged unless one of them is given.
	BinaryDefault BinaryMode = iota
	// BinaryText (-a/--text) treats a binary-detected file exactly like
	// text: matching, context lines (-A/-B/-C), -v, and normal output
	// all behave as if the file were never flagged binary at all — raw
	// NUL bytes and all. Verified directly against real GNU grep, not
	// assumed.
	BinaryText
	// BinarySkip (-I) treats a binary-detected file as if it produced no
	// matches at all, without reading any further — verified directly
	// against real GNU grep: -c reports 0 for it, -l never lists it,
	// -L always does, and -v makes no difference (the file is simply
	// never searched).
	BinarySkip
)
