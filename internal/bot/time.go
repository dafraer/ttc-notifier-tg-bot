package bot

import (
	"fmt"
	"time"
)

// Tbilisi is fixed at UTC+4 all year (Georgia observes no daylight saving),
// so we use a fixed zone and avoid depending on the tzdata files being present
// in the container image.
var Tbilisi = time.FixedZone("Asia/Tbilisi", 4*3600)

const (
	slotMinutes = 30 // size of a time slot
	totalSlots  = (24 * 60) / slotMinutes
	windowSize  = 5 // time slots shown at once in the picker
)

// formatMinutes renders a minutes-of-day value as "HH:MM".
func formatMinutes(m int) string {
	m = ((m % (24 * 60)) + 24*60) % (24 * 60)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// NowMinutes returns the current minutes-of-day in Tbilisi time.
func NowMinutes(now time.Time) int {
	t := now.In(Tbilisi)
	return t.Hour()*60 + t.Minute()
}

// WithinWindow reports whether minutes-of-day `now` falls inside the [start,end]
// window. If end < start the window is treated as wrapping past midnight.
func WithinWindow(now, start, end int) bool {
	if start <= end {
		return now >= start && now <= end
	}
	return now >= start || now <= end
}
