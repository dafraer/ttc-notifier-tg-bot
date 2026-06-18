package bot

import "testing"

func TestWithinWindow(t *testing.T) {
	const (
		h8  = 8 * 60
		h9  = 9 * 60
		h22 = 22 * 60
		h2  = 2 * 60
	)
	tests := []struct {
		name            string
		now, start, end int
		want            bool
	}{
		{"inside normal window", h8 + 30, h8, h9, true},
		{"at start boundary", h8, h8, h9, true},
		{"at end boundary", h9, h8, h9, true},
		{"before normal window", h8 - 1, h8, h9, false},
		{"after normal window", h9 + 1, h8, h9, false},
		{"overnight: late evening", h22 + 30, h22, h2, true},
		{"overnight: early morning", h2 - 30, h22, h2, true},
		{"overnight: midday excluded", 12 * 60, h22, h2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WithinWindow(tt.now, tt.start, tt.end); got != tt.want {
				t.Fatalf("WithinWindow(%d,%d,%d) = %v, want %v", tt.now, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestFormatMinutes(t *testing.T) {
	cases := map[int]string{0: "00:00", 90: "01:30", 1410: "23:30", 1439: "23:59"}
	for in, want := range cases {
		if got := formatMinutes(in); got != want {
			t.Errorf("formatMinutes(%d) = %q, want %q", in, got, want)
		}
	}
}
