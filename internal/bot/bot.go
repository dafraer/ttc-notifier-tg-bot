// Package bot implements the Telegram interface: the /add and /remove wizards
// and the inline-keyboard interactions backing them.
package bot

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"tbilisi-transport-tg-bot/internal/storage"
	"tbilisi-transport-tg-bot/ttc"
)

const (
	maxStopResults  = 8
	stopCacheTTL    = 10 * time.Minute
	promptReminder  = "How long before arrival should I remind you? ⏰"
	promptStart     = "🕒 Pick the <b>start</b> of the notification window:"
	promptEnd       = "🕓 Pick the <b>end</b> of the notification window:"
	promptName      = "✏️ Almost done! Send a <b>name</b> for this reminder (e.g. \"Morning commute\")."
	promptStop      = "🚏 Send the <b>name of the stop</b> where you'll be waiting, and I'll search for it."
	promptBusNumber = "🚌 Send the <b>bus number</b> you want to track."
)

// Bot wires the Telegram bot to storage and the TTC API.
type Bot struct {
	store   *storage.Store
	ttc     *ttc.Client
	log     *zap.Logger
	api     *bot.Bot
	trigger chan<- struct{}

	stopMu  sync.Mutex
	stops   []ttc.BusStop
	stopsAt time.Time

	routeMu  sync.Mutex
	routes   []ttc.Bus
	routesAt time.Time
}

// New creates the bot and registers all handlers. trigger is signalled (best
// effort) whenever a new notification is added, so the notifier can check
// immediately rather than waiting for its next tick.
func New(token string, store *storage.Store, ttcClient *ttc.Client, log *zap.Logger, trigger chan<- struct{}) (*Bot, error) {
	b := &Bot{store: store, ttc: ttcClient, log: log, trigger: trigger}

	opts := []bot.Option{bot.WithDefaultHandler(b.onText)}
	api, err := bot.New(token, opts...)
	if err != nil {
		return nil, err
	}
	b.api = api

	api.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.cmdHelp)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, b.cmdHelp)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/add", bot.MatchTypeExact, b.cmdAdd)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/remove", bot.MatchTypeExact, b.cmdRemove)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/list", bot.MatchTypeExact, b.cmdList)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/cancel", bot.MatchTypeExact, b.cmdCancel)
	// One catch-all callback handler; we dispatch on the data prefix ourselves.
	api.RegisterHandler(bot.HandlerTypeCallbackQueryData, "", bot.MatchTypePrefix, b.onCallback)

	return b, nil
}

// API returns the underlying telegram bot, used by the notifier to send messages.
func (b *Bot) API() *bot.Bot { return b.api }

// Start runs the bot's long-polling loop until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) { b.api.Start(ctx) }

// SendReminder delivers an arrival reminder for a notification. It is used by
// the notifier package.
func (b *Bot) SendReminder(ctx context.Context, n *storage.Notification, minutes int) {
	b.send(ctx, n.ChatID, reminderMessage(n, minutes), nil)
}

// ---- helpers ----

func emptyKB() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
}

func (b *Bot) send(ctx context.Context, chatID int64, text string, kb models.ReplyMarkup) {
	_, err := b.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: kb,
	})
	if err != nil {
		b.log.Warn("send message failed", zap.Int64("chat", chatID), zap.Error(err))
	}
}

func (b *Bot) edit(ctx context.Context, chatID int64, messageID int, text string, kb models.ReplyMarkup) {
	_, err := b.api.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: kb,
	})
	if err != nil {
		b.log.Warn("edit message failed", zap.Int64("chat", chatID), zap.Error(err))
	}
}

// ---- commands ----

// sender extracts the user and chat ids from a message update. It returns
// ok=false when there is no identifiable sender (e.g. an anonymous group admin
// or channel post, where From is nil), so callers can skip safely.
func sender(u *models.Update) (userID, chatID int64, ok bool) {
	if u.Message == nil || u.Message.From == nil {
		return 0, 0, false
	}
	return u.Message.From.ID, u.Message.Chat.ID, true
}

func (b *Bot) cmdHelp(ctx context.Context, _ *bot.Bot, u *models.Update) {
	if u.Message == nil {
		return
	}
	text := strings.Join([]string{
		"👋 <b>Tbilisi Transport reminder bot</b>",
		"",
		"I watch the TTC arrival times and ping you before your bus shows up.",
		"",
		"<b>Commands</b>",
		"/add — create a new bus reminder",
		"/list — show your reminders",
		"/remove — delete a reminder",
		"/cancel — abort the current /add wizard",
	}, "\n")
	b.send(ctx, u.Message.Chat.ID, text, nil)
}

func (b *Bot) cmdAdd(ctx context.Context, _ *bot.Bot, u *models.Update) {
	userID, chatID, ok := sender(u)
	if !ok {
		return
	}
	st := &storage.State{
		UserID: userID,
		ChatID: chatID,
		Step:   storage.StepAwaitingBus,
	}
	if err := b.store.SaveState(st); err != nil {
		b.log.Error("save state", zap.Error(err))
		b.send(ctx, chatID, "⚠️ Something went wrong, please try again.", nil)
		return
	}
	b.send(ctx, chatID, promptBusNumber, nil)
}

func (b *Bot) cmdCancel(ctx context.Context, _ *bot.Bot, u *models.Update) {
	userID, chatID, ok := sender(u)
	if !ok {
		return
	}
	_ = b.store.ClearState(userID)
	b.send(ctx, chatID, "❌ Cancelled.", nil)
}

func (b *Bot) cmdList(ctx context.Context, _ *bot.Bot, u *models.Update) {
	userID, chatID, ok := sender(u)
	if !ok {
		return
	}
	ns, err := b.store.ListNotifications(userID)
	if err != nil {
		b.log.Error("list notifications", zap.Error(err))
		return
	}
	if len(ns) == 0 {
		b.send(ctx, chatID, "You have no reminders yet. Use /add to create one.", nil)
		return
	}
	var parts []string
	for i := range ns {
		parts = append(parts, notificationCard(&ns[i]))
	}
	b.send(ctx, chatID, strings.Join(parts, "\n\n➖➖➖\n\n"), nil)
}

func (b *Bot) cmdRemove(ctx context.Context, _ *bot.Bot, u *models.Update) {
	userID, chatID, ok := sender(u)
	if !ok {
		return
	}
	ns, err := b.store.ListNotifications(userID)
	if err != nil {
		b.log.Error("list notifications", zap.Error(err))
		return
	}
	if len(ns) == 0 {
		b.send(ctx, chatID, "You have no reminders to remove.", nil)
		return
	}
	b.send(ctx, chatID, "🗑 Which reminder should I remove?", removeKeyboard(ns))
}

// ---- text (wizard) ----

func (b *Bot) onText(ctx context.Context, _ *bot.Bot, u *models.Update) {
	if u.Message == nil || u.Message.From == nil {
		return
	}
	userID := u.Message.From.ID
	chatID := u.Message.Chat.ID
	text := strings.TrimSpace(u.Message.Text)

	st, err := b.store.GetState(userID)
	if err != nil {
		b.log.Error("get state", zap.Error(err))
		return
	}
	if st == nil || st.Step == storage.StepIdle {
		b.send(ctx, chatID, "Use /add to create a reminder, or /help to see what I can do.", nil)
		return
	}

	switch st.Step {
	case storage.StepAwaitingBus:
		if text == "" {
			b.send(ctx, chatID, promptBusNumber, nil)
			return
		}
		exists, canonical, err := b.busExists(ctx, text)
		if err != nil {
			b.log.Error("check bus exists", zap.Error(err))
			b.send(ctx, chatID, "⚠️ Couldn't reach the transport service, please try again in a moment.", nil)
			return
		}
		if !exists {
			b.send(ctx, chatID, "🚫 I couldn't find bus <b>"+esc(text)+"</b>. Check the number and send it again.", nil)
			return
		}
		st.BusNumber = canonical
		st.Step = storage.StepAwaitingStopText
		b.saveState(ctx, chatID, st)
		b.send(ctx, chatID, promptStop, nil)

	case storage.StepAwaitingStopText:
		choices, err := b.searchStops(ctx, text)
		if err != nil {
			b.log.Error("search stops", zap.Error(err))
			b.send(ctx, chatID, "⚠️ Couldn't reach the transport service, please try again in a moment.", nil)
			return
		}
		if len(choices) == 0 {
			b.send(ctx, chatID, "🤷 No stops matched that. Try a different name.", nil)
			return
		}
		raw, _ := json.Marshal(choices)
		st.StopResults = string(raw)
		st.Step = storage.StepAwaitingStopPick
		b.saveState(ctx, chatID, st)
		b.send(ctx, chatID, "📍 Pick your stop:", stopsKeyboard(choices))

	case storage.StepAwaitingName:
		if text == "" {
			b.send(ctx, chatID, promptName, nil)
			return
		}
		b.finishWizard(ctx, chatID, st, text)

	default:
		b.send(ctx, chatID, "👆 Please use the buttons above, or /cancel to start over.", nil)
	}
}

func (b *Bot) finishWizard(ctx context.Context, chatID int64, st *storage.State, name string) {
	n := &storage.Notification{
		UserID:          st.UserID,
		ChatID:          st.ChatID,
		Name:            name,
		BusNumber:       st.BusNumber,
		StopID:          st.StopID,
		StopName:        st.StopName,
		StopCode:        st.StopCode,
		ReminderMinutes: st.ReminderMinutes,
		StartMinutes:    st.StartMinutes,
		EndMinutes:      st.EndMinutes,
	}
	id, err := b.store.AddNotification(n)
	if err != nil {
		b.log.Error("add notification", zap.Error(err))
		b.send(ctx, chatID, "⚠️ Couldn't save the reminder, please try again.", nil)
		return
	}
	n.ID = id
	_ = b.store.ClearState(st.UserID)

	// Let the notifier know there's new work right away.
	if b.trigger != nil {
		select {
		case b.trigger <- struct{}{}:
		default:
		}
	}

	b.send(ctx, chatID, "✅ <b>Reminder created!</b>\n\n"+notificationCard(n), nil)
}

// ---- callbacks ----

func (b *Bot) onCallback(ctx context.Context, _ *bot.Bot, u *models.Update) {
	cq := u.CallbackQuery
	if cq == nil {
		return
	}
	defer func() {
		_, _ = b.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cq.ID})
	}()

	if cq.Data == "noop" {
		return
	}

	// We always edit the originating message; if it's inaccessible (too old)
	// there's nothing we can act on.
	if cq.Message.Message == nil {
		return
	}
	chatID := cq.Message.Message.Chat.ID
	messageID := cq.Message.Message.ID
	userID := cq.From.ID

	parts := strings.Split(cq.Data, ":")
	switch parts[0] {
	case "rem":
		b.cbReminder(ctx, userID, chatID, messageID, parts)
	case "nav":
		b.cbNav(ctx, chatID, messageID, parts)
	case "pick":
		b.cbPick(ctx, userID, chatID, messageID, parts)
	case "stop":
		b.cbStop(ctx, userID, chatID, messageID, parts)
	case "del":
		b.cbDelete(ctx, userID, chatID, messageID, parts)
	}
}

func (b *Bot) cbReminder(ctx context.Context, userID, chatID int64, messageID int, parts []string) {
	st := b.requireStep(ctx, userID, chatID, messageID, storage.StepAwaitingReminder)
	if st == nil || len(parts) < 2 {
		return
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	st.ReminderMinutes = m
	st.Step = storage.StepAwaitingStart
	b.saveState(ctx, chatID, st)
	b.edit(ctx, chatID, messageID, promptStart, timeKeyboard("start", 0))
}

func (b *Bot) cbNav(ctx context.Context, chatID int64, messageID int, parts []string) {
	if len(parts) < 3 {
		return
	}
	kind := parts[1]
	offset, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	prompt := promptStart
	if kind == "end" {
		prompt = promptEnd
	}
	b.edit(ctx, chatID, messageID, prompt, timeKeyboard(kind, offset))
}

func (b *Bot) cbPick(ctx context.Context, userID, chatID int64, messageID int, parts []string) {
	if len(parts) < 3 {
		return
	}
	kind := parts[1]
	m, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	switch kind {
	case "start":
		st := b.requireStep(ctx, userID, chatID, messageID, storage.StepAwaitingStart)
		if st == nil {
			return
		}
		st.StartMinutes = m
		st.Step = storage.StepAwaitingEnd
		b.saveState(ctx, chatID, st)
		b.edit(ctx, chatID, messageID, promptEnd, timeKeyboard("end", 0))
	case "end":
		st := b.requireStep(ctx, userID, chatID, messageID, storage.StepAwaitingEnd)
		if st == nil {
			return
		}
		st.EndMinutes = m
		st.Step = storage.StepAwaitingName
		b.saveState(ctx, chatID, st)
		summary := "🕒 Window: <b>" + formatMinutes(st.StartMinutes) + "–" + formatMinutes(st.EndMinutes) + "</b>"
		b.edit(ctx, chatID, messageID, summary, emptyKB())
		b.send(ctx, chatID, promptName, nil)
	}
}

func (b *Bot) cbStop(ctx context.Context, userID, chatID int64, messageID int, parts []string) {
	st := b.requireStep(ctx, userID, chatID, messageID, storage.StepAwaitingStopPick)
	if st == nil || len(parts) < 2 {
		return
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	var choices []storage.StopChoice
	if err := json.Unmarshal([]byte(st.StopResults), &choices); err != nil || idx < 0 || idx >= len(choices) {
		return
	}
	choice := choices[idx]
	st.StopID = choice.ID
	st.StopName = choice.Name
	st.StopCode = choice.Code
	st.Step = storage.StepAwaitingReminder
	b.saveState(ctx, chatID, st)
	b.edit(ctx, chatID, messageID, "📍 Stop: <b>"+esc(stopLabel(choice.Name, choice.Code))+"</b>\n\n"+promptReminder, reminderKeyboard())
}

func (b *Bot) cbDelete(ctx context.Context, userID, chatID int64, messageID int, parts []string) {
	if len(parts) < 2 {
		return
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	ok, err := b.store.DeleteNotification(id, userID)
	if err != nil {
		b.log.Error("delete notification", zap.Error(err))
		return
	}
	if ok {
		b.edit(ctx, chatID, messageID, "🗑 Reminder removed.", emptyKB())
	} else {
		b.edit(ctx, chatID, messageID, "That reminder no longer exists.", emptyKB())
	}
}

// requireStep loads the user's state and verifies it is at the expected step.
// On mismatch it notifies the user (the wizard likely expired) and returns nil.
func (b *Bot) requireStep(ctx context.Context, userID, chatID int64, messageID int, step string) *storage.State {
	st, err := b.store.GetState(userID)
	if err != nil {
		b.log.Error("get state", zap.Error(err))
		return nil
	}
	if st == nil || st.Step != step {
		b.edit(ctx, chatID, messageID, "⌛ This step expired. Use /add to start again.", emptyKB())
		return nil
	}
	return st
}

func (b *Bot) saveState(ctx context.Context, chatID int64, st *storage.State) {
	if err := b.store.SaveState(st); err != nil {
		b.log.Error("save state", zap.Error(err))
		b.send(ctx, chatID, "⚠️ Something went wrong, please try /cancel and /add again.", nil)
	}
}

// ---- stop search ----

// searchStops finds stops whose name contains the query (case-insensitive),
// using a short-lived in-memory cache of the full stop list.
func (b *Bot) searchStops(ctx context.Context, query string) ([]storage.StopChoice, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil, nil
	}

	stops, err := b.loadStops(ctx)
	if err != nil {
		return nil, err
	}

	var out []storage.StopChoice
	for _, s := range stops {
		if !strings.Contains(strings.ToLower(s.Name), query) {
			continue
		}
		code := ""
		if s.Code != nil {
			code = *s.Code
		}
		// The /stops endpoint returns ids already prefixed with the feed id
		// (e.g. "1:925"), but the ttc client prepends "1:" itself, so we store
		// the bare id to avoid a double prefix.
		out = append(out, storage.StopChoice{ID: bareStopID(s.ID), Name: s.Name, Code: code})
		if len(out) >= maxStopResults {
			break
		}
	}
	return out, nil
}

// bareStopID strips the leading feed prefix (e.g. "1:925" -> "925") so the id
// can be passed to the ttc client, which adds the "1:" prefix itself.
func bareStopID(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
}

// busExists reports whether a bus with the given short name exists. On a match
// it also returns the route's canonical short name (so casing/spacing is
// normalized to what the TTC API uses).
func (b *Bot) busExists(ctx context.Context, busNumber string) (bool, string, error) {
	want := strings.TrimSpace(strings.ToLower(busNumber))
	routes, err := b.loadRoutes(ctx)
	if err != nil {
		return false, "", err
	}
	for _, r := range routes {
		if strings.ToLower(strings.TrimSpace(r.ShortName)) == want {
			return true, r.ShortName, nil
		}
	}
	return false, "", nil
}

func (b *Bot) loadRoutes(ctx context.Context) ([]ttc.Bus, error) {
	b.routeMu.Lock()
	defer b.routeMu.Unlock()

	if b.routes != nil && time.Since(b.routesAt) < stopCacheTTL {
		return b.routes, nil
	}
	routes, err := b.ttc.Routes(ctx, "")
	if err != nil {
		return nil, err
	}
	b.routes = routes
	b.routesAt = time.Now()
	return routes, nil
}

func (b *Bot) loadStops(ctx context.Context) ([]ttc.BusStop, error) {
	b.stopMu.Lock()
	defer b.stopMu.Unlock()

	if b.stops != nil && time.Since(b.stopsAt) < stopCacheTTL {
		return b.stops, nil
	}
	stops, err := b.ttc.Stops(ctx, "")
	if err != nil {
		return nil, err
	}
	b.stops = stops
	b.stopsAt = time.Now()
	return stops, nil
}
