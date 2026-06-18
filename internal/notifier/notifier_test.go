package notifier

import (
	"testing"

	"tbilisi-transport-tg-bot/ttc"
)

func TestSoonestArrival(t *testing.T) {
	arrivals := []ttc.BusArrival{
		{ShortName: "305", Realtime: true, RealtimeArrivalMinutes: 1},
		{ShortName: "385", Realtime: true, RealtimeArrivalMinutes: 8},
		{ShortName: "385", Realtime: true, RealtimeArrivalMinutes: 3},   // soonest 385
		{ShortName: "385", Realtime: false, RealtimeArrivalMinutes: 0},  // must be ignored
		{ShortName: "385", Realtime: false, RealtimeArrivalMinutes: -1}, // must be ignored
	}

	tests := []struct {
		name   string
		bus    string
		want   int
		wantOK bool
	}{
		{"matches soonest realtime", "385", 3, true},
		{"case insensitive", " 305 ", 1, true},
		{"no match", "999", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := soonestArrival(tt.bus, arrivals)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("soonestArrival(%q) = (%d, %v), want (%d, %v)", tt.bus, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSoonestArrivalOnlyNonRealtime(t *testing.T) {
	// A scheduled-only entry with a sentinel/zero value must not fire.
	arrivals := []ttc.BusArrival{
		{ShortName: "385", Realtime: false, RealtimeArrivalMinutes: 0},
	}
	if _, ok := soonestArrival("385", arrivals); ok {
		t.Fatal("expected no live arrival for scheduled-only entry")
	}
}

func TestBareStopID(t *testing.T) {
	cases := map[string]string{"1:925": "925", "925": "925", "1:1:5": "1:5", "": ""}
	for in, want := range cases {
		if got := bareStopID(in); got != want {
			t.Errorf("bareStopID(%q) = %q, want %q", in, got, want)
		}
	}
}
