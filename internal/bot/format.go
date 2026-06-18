package bot

import (
	"fmt"
	"strings"

	"tbilisi-transport-tg-bot/internal/storage"
)

// stopLabel formats a stop as "Name (code)", or just the name when no code is
// known. The result is NOT HTML-escaped — callers must escape it if needed.
func stopLabel(name, code string) string {
	if code == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, code)
}

// esc escapes text for Telegram's HTML parse mode.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// notificationCard renders a notification as a pretty HTML block.
func notificationCard(n *storage.Notification) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 <b>%s</b>\n", esc(n.Name))
	fmt.Fprintf(&b, "🚌 Bus: <b>%s</b>\n", esc(n.BusNumber))
	fmt.Fprintf(&b, "📍 Stop: %s\n", esc(stopLabel(n.StopName, n.StopCode)))
	fmt.Fprintf(&b, "⏰ Remind: <b>%d min</b> before arrival\n", n.ReminderMinutes)
	fmt.Fprintf(&b, "🕒 Active: <b>%s–%s</b>", formatMinutes(n.StartMinutes), formatMinutes(n.EndMinutes))
	return b.String()
}

// reminderMessage renders the message sent when a bus is approaching.
func reminderMessage(n *storage.Notification, minutes int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🚍 <b>Bus %s</b> is arriving!\n\n", esc(n.BusNumber))
	if minutes <= 0 {
		b.WriteString("It is arriving <b>now</b> ")
	} else {
		fmt.Fprintf(&b, "Arriving in <b>%d min</b> ", minutes)
	}
	fmt.Fprintf(&b, "at <b>%s</b>.\n", esc(stopLabel(n.StopName, n.StopCode)))
	fmt.Fprintf(&b, "\n<i>Reminder: %s</i>", esc(n.Name))
	return b.String()
}
