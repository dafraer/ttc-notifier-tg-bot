// Command tbilisi-transport-tg-bot is a Telegram bot that reminds users before
// their bus arrives at a chosen stop, using the Tbilisi Transport Company API.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	tbot "tbilisi-transport-tg-bot/internal/bot"
	"tbilisi-transport-tg-bot/internal/notifier"
	"tbilisi-transport-tg-bot/internal/storage"
	"tbilisi-transport-tg-bot/ttc"
)

func main() {
	// Load .env if present; ignore the error so real env vars still work.
	_ = godotenv.Load()

	log, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "data/bot.db"
	}

	locale := os.Getenv("TTC_LOCALE")
	if locale == "" {
		locale = ttc.LocaleEn
	}

	store, err := storage.Open(dbPath)
	if err != nil {
		log.Fatal("open storage", zap.Error(err))
	}
	defer store.Close()

	ttcClient := ttc.New()
	ttcClient.SetLocale(locale)

	// Buffered so the bot never blocks when nudging the notifier.
	trigger := make(chan struct{}, 1)

	b, err := tbot.New(token, store, ttcClient, log, trigger)
	if err != nil {
		log.Fatal("create bot", zap.Error(err))
	}

	n := notifier.New(store, ttcClient, b, log, trigger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go n.Run(ctx)

	log.Info("bot started")
	b.Start(ctx) // blocks until ctx is cancelled
	log.Info("bot stopped")
}
