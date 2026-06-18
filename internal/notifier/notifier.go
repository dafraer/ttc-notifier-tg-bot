// Package notifier polls the TTC API and sends arrival reminders.
package notifier

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	tbot "tbilisi-transport-tg-bot/internal/bot"
	"tbilisi-transport-tg-bot/internal/storage"
	"tbilisi-transport-tg-bot/ttc"
)

// cooldown is the minimum time between two reminders for the same notification,
// so a user isn't pinged every minute while the bus approaches.
const cooldown = 12 * time.Minute

// Notifier checks the TTC API once a minute and sends reminders.
type Notifier struct {
	store   *storage.Store
	ttc     *ttc.Client
	tg      *tbot.Bot
	log     *zap.Logger
	trigger <-chan struct{}
}

// New creates a Notifier. trigger lets the bot wake it up immediately when a
// new notification is added.
func New(store *storage.Store, ttcClient *ttc.Client, tg *tbot.Bot, log *zap.Logger, trigger <-chan struct{}) *Notifier {
	return &Notifier{store: store, ttc: ttcClient, tg: tg, log: log, trigger: trigger}
}

// Run polls every minute until ctx is cancelled. It also reacts to the trigger
// channel so freshly added notifications are picked up without delay.
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	n.check(ctx) // initial pass on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.check(ctx)
		case <-n.trigger:
			n.check(ctx)
		}
	}
}

// check loads every notification and sends reminders for buses that are due.
func (n *Notifier) check(ctx context.Context) {
	notifs, err := n.store.AllNotifications()
	if err != nil {
		n.log.Error("notifier: load notifications", zap.Error(err))
		return
	}

	now := time.Now()
	nowMin := tbot.NowMinutes(now)

	// Group notifications by stop so we hit the arrival-times endpoint once per
	// stop rather than once per notification. The key is the bare id (the ttc
	// client adds the "1:" prefix itself) so stops stored either as "925" or an
	// older "1:925" form collapse into a single request.
	byStop := make(map[string][]storage.Notification)
	for _, no := range notifs {
		if !tbot.WithinWindow(nowMin, no.StartMinutes, no.EndMinutes) {
			continue
		}
		key := bareStopID(no.StopID)
		byStop[key] = append(byStop[key], no)
	}

	for stopID, group := range byStop {
		select {
		case <-ctx.Done():
			return
		default:
		}

		arrivals, err := n.ttc.ArrivalTimes(ctx, ttc.ArrivalOptions{StopID: stopID})
		if err != nil {
			n.log.Warn("notifier: arrival times", zap.String("stop", stopID), zap.Error(err))
			continue
		}

		for i := range group {
			n.maybeNotify(ctx, &group[i], arrivals, now)
		}
	}
}

func (n *Notifier) maybeNotify(ctx context.Context, no *storage.Notification, arrivals []ttc.BusArrival, now time.Time) {
	// Respect the cooldown so we don't spam every minute.
	if no.LastNotifiedAt.Valid {
		last := time.Unix(no.LastNotifiedAt.Int64, 0)
		if now.Sub(last) < cooldown {
			return
		}
	}

	minutes, ok := soonestArrival(no.BusNumber, arrivals)
	if !ok {
		return
	}
	if minutes > no.ReminderMinutes {
		return // still too far away
	}

	n.tg.SendReminder(ctx, no, minutes)
	if err := n.store.SetLastNotified(no.ID, now.Unix()); err != nil {
		n.log.Error("notifier: set last notified", zap.Int64("id", no.ID), zap.Error(err))
	}
	n.log.Info("reminder sent",
		zap.Int64("id", no.ID),
		zap.String("bus", no.BusNumber),
		zap.String("stop", no.StopName),
		zap.Int("minutes", minutes))
}

// bareStopID strips a leading feed prefix (e.g. "1:925" -> "925"). The ttc
// client adds the "1:" prefix itself, so ids must be passed without one.
func bareStopID(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
}

// soonestArrival returns the smallest live arrival time (in minutes) for the
// given bus short name among the arrivals, and whether any matched.
//
// Only entries with realtime data are considered: for scheduled-only entries
// the API reports realtime=false and realtimeArrivalMinutes is not a live
// prediction (it can be 0 or a sentinel like -1), which would otherwise fire a
// bogus "arriving now" reminder.
func soonestArrival(busNumber string, arrivals []ttc.BusArrival) (int, bool) {
	want := strings.TrimSpace(strings.ToLower(busNumber))
	best := -1
	for _, a := range arrivals {
		if strings.ToLower(strings.TrimSpace(a.ShortName)) != want {
			continue
		}
		if !a.Realtime || a.RealtimeArrivalMinutes < 0 {
			continue
		}
		m := a.RealtimeArrivalMinutes
		if best == -1 || m < best {
			best = m
		}
	}
	if best == -1 {
		return 0, false
	}
	return best, true
}
