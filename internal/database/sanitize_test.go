package database

import (
	"bytes"
	"testing"
)

// JSON escapes are written in raw string literals: in an interpreted Go string, \u0000
// would be a real NUL byte instead of the six-character escape. Raw invalid bytes are
// written with \x in interpreted strings.
func TestSanitizePayload(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte // nil means the input, unchanged
		wantN repairCounts
	}{
		// Repairable cases from the probe table.
		{"NUL in value", []byte(`{"a":"x\u0000y"}`), []byte(`{"a":"x\uFFFDy"}`), repairCounts{1, 0, 0}},
		{"NUL in key", []byte(`{"\u0000":1}`), []byte(`{"\uFFFD":1}`), repairCounts{1, 0, 0}},
		{"lone high", []byte(`"\ud83d"`), []byte(`"\uFFFD"`), repairCounts{0, 1, 0}},
		{"lone low", []byte(`"\ude00"`), []byte(`"\uFFFD"`), repairCounts{0, 1, 0}},
		{"reversed pair", []byte(`"\ude00\ud83d"`), []byte(`"\uFFFD\uFFFD"`), repairCounts{0, 2, 0}},
		{"raw 0xff", []byte("x\xffy"), []byte("x\uFFFDy"), repairCounts{0, 0, 1}},
		{"CESU-8 surrogate", []byte("x\xed\xa0\xbdy"), []byte("x\uFFFDy"), repairCounts{0, 0, 1}},
		{"overlong", []byte("x\xc0\xafy"), []byte("x\uFFFDy"), repairCounts{0, 0, 1}},
		{"two separate runs", []byte("\xffa\xfe"), []byte("\uFFFDa\uFFFD"), repairCounts{0, 0, 2}},

		// Backslash runs before u0000: an odd run ends with an escape start.
		{"1 backslash", []byte(`"\u0000"`), []byte(`"\uFFFD"`), repairCounts{1, 0, 0}},
		{"2 backslashes", []byte(`"\\u0000"`), nil, repairCounts{}},
		{"3 backslashes", []byte(`"\\\u0000"`), []byte(`"\\\uFFFD"`), repairCounts{1, 0, 0}},
		{"4 backslashes", []byte(`"\\\\u0000"`), nil, repairCounts{}},

		// End of input.
		{"NUL at the very end", []byte(`x\u0000`), []byte(`x\uFFFD`), repairCounts{1, 0, 0}},
		{"lone high at the very end", []byte(`x\ud83d`), []byte(`x\uFFFD`), repairCounts{0, 1, 0}},
		{"truncated escape", []byte(`x\u00`), nil, repairCounts{}},
		{"truncated escape of three digits", []byte(`x\u000`), nil, repairCounts{}},
		{"escape with a non-hex digit", []byte(`x\u00g0`), nil, repairCounts{}},
		{"trailing backslash", []byte(`x\`), nil, repairCounts{}},
		{"high surrogate then truncated escape", []byte(`x\ud83d\ude0`), []byte(`x\uFFFD\ude0`), repairCounts{0, 1, 0}},
		{"backslash then invalid byte", []byte("x\\\xff"), []byte("x\\\uFFFD"), repairCounts{0, 0, 1}},

		// Surrogate sequences.
		{"high then escaped backslash", []byte(`"\ud83d\\uDE00"`), []byte(`"\uFFFD\\uDE00"`), repairCounts{0, 1, 0}},
		{"high, high, low", []byte(`"\ud83d\ud83d\ude00"`), []byte(`"\uFFFD\ud83d\ude00"`), repairCounts{0, 1, 0}},
		{"high then NUL", []byte(`"\ud83d\u0000"`), []byte(`"\uFFFD\uFFFD"`), repairCounts{1, 1, 0}},
		{"lone high, upper-case hex", []byte(`"\uDBFF"`), []byte(`"\uFFFD"`), repairCounts{0, 1, 0}},
		{"lone low, mixed-case hex", []byte(`"\uDc00"`), []byte(`"\uFFFD"`), repairCounts{0, 1, 0}},

		// Several defects in one payload.
		{
			"several defects",
			[]byte(`{"a":"\u0000","b":"\ude00","c":"` + "\xff" + `","d":"x\u0000"}`),
			[]byte(`{"a":"\uFFFD","b":"\uFFFD","c":"` + "\uFFFD" + `","d":"x\uFFFD"}`),
			repairCounts{2, 1, 1},
		},

		// Accepted by PostgreSQL: passed through unchanged and uncounted.
		{"controls and noncharacter", []byte(`"\u0001\u001F\uFFFF"`), nil, repairCounts{}},
		{"valid pair", []byte(`"\ud83d\ude00"`), nil, repairCounts{}},
		{"valid pair, upper-case hex", []byte(`"\uD83D\uDE00"`), nil, repairCounts{}},
		{"valid pair, mixed-case hex", []byte(`"\uD83d\ude00"`), nil, repairCounts{}},
		{"valid pair, upper high and lower low", []byte(`"\uD83D\ude00"`), nil, repairCounts{}},
		{"other escapes", []byte(`"\"\\\/\b\f\n\r\t"`), nil, repairCounts{}},
		{"literal U+FFFD and multi-byte UTF-8", []byte("\"é 😀 \uFFFD\""), nil, repairCounts{}},
		{"numeric overflow", []byte(`{"v":1e1000000}`), nil, repairCounts{}},
		{"not JSON", []byte(`online`), nil, repairCounts{}},
		{"truncated JSON", []byte(`{"a":`), nil, repairCounts{}},
		{"raw NUL byte", []byte("\"x\x00y\""), nil, repairCounts{}},
		{"empty", []byte{}, nil, repairCounts{}},
		{"nil", nil, nil, repairCounts{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = tt.input
			}

			got, counts := sanitizePayload(tt.input)

			if !bytes.Equal(got, want) {
				t.Errorf("sanitizePayload(%q) = %q, want %q", tt.input, got, want)
			}
			if counts.nulEscapes != tt.wantN.nulEscapes {
				t.Errorf("nulEscapes = %d, want %d", counts.nulEscapes, tt.wantN.nulEscapes)
			}
			if counts.loneSurrogates != tt.wantN.loneSurrogates {
				t.Errorf("loneSurrogates = %d, want %d", counts.loneSurrogates, tt.wantN.loneSurrogates)
			}
			if counts.invalidUTF8Sequences != tt.wantN.invalidUTF8Sequences {
				t.Errorf("invalidUTF8Sequences = %d, want %d", counts.invalidUTF8Sequences, tt.wantN.invalidUTF8Sequences)
			}
			if tt.want == nil && (len(got) != len(tt.input) || (len(got) > 0 && &got[0] != &tt.input[0])) {
				t.Errorf("sanitizePayload(%q) returned a new slice, want the input itself", tt.input)
			}
		})
	}
}

// TestSanitizePayload_NoAllocOnCleanPayload runs on every message, so a payload needing
// no repair must cost no allocation. The payload goes through every non-defect branch.
// No t.Parallel: AllocsPerRun counts allocations process-wide.
func TestSanitizePayload_NoAllocOnCleanPayload(t *testing.T) {
	payload := []byte(`{"path":"C:\\u0000","quote":"\"x\"","ctrl":"\u0001","emoji":"\ud83d\ude00","text":"` +
		"é 😀 \uFFFD" + `","n":[1,2.5e3,true,null]}`)

	var got []byte
	var counts repairCounts
	allocs := testing.AllocsPerRun(100, func() {
		got, counts = sanitizePayload(payload)
	})

	if allocs != 0 {
		t.Errorf("sanitizePayload allocated %v times per call on a clean payload, want 0", allocs)
	}
	if counts.total() != 0 {
		t.Errorf("counts = %+v, want none", counts)
	}
	if len(got) != len(payload) || &got[0] != &payload[0] {
		t.Error("sanitizePayload returned a new slice for a clean payload, want the input itself")
	}
}
