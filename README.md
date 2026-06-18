# 🚌 Tbilisi Transport Reminder Bot

A Telegram bot that reminds you before your bus arrives at a chosen stop in
Tbilisi, Georgia. It polls the [Tbilisi Transport Company (TTC)][ttc] real-time
arrival API once a minute and pings you when a tracked bus is approaching.

Built with:

- [`github.com/go-telegram/bot`](https://github.com/go-telegram/bot) — Telegram Bot API
- [`go.uber.org/zap`](https://github.com/uber-go/zap) — structured logging
- [`github.com/joho/godotenv`](https://github.com/joho/godotenv) — `.env` loading
- [`modernc.org/sqlite`](https://modernc.org/sqlite) — pure-Go SQLite (no CGO)

## Features

- ⏰ Per-bus arrival reminders with a configurable lead time (5–30 minutes).
- 🕒 A daily time window during which each reminder is active, chosen with a
  scrollable 30-minute interval picker.
- 🏷️ A custom name for every reminder.
- 💾 All conversation state and reminders are persisted in SQLite, so restarts
  are safe.
- 🔁 A background notifier goroutine checks the TTC API every minute and is
  nudged immediately whenever a new reminder is added.

## Project layout

```
.
├── main.go                  # entry point: wiring, config, lifecycle
├── ttc/                     # Go port of the ttc-api library (complete copy)
│   ├── ttc.go               # the API client
│   └── types.go             # response types
└── internal/
    ├── storage/             # SQLite persistence (wizard state + notifications)
    ├── bot/                 # Telegram handlers, keyboards, formatting
    └── notifier/            # polling goroutine that sends reminders
```

The `ttc` package is a faithful Go translation of the TypeScript `ttc-api`
library found in `./ttc-api`, exposing the same endpoints: `Stops`, `Stop`,
`Routes`, `Plan`, `BusPolyline`, `Locations`, `StopRoutes`, `BusRoutes` and
`ArrivalTimes`.

## How it works

### `/add` — create a reminder

The bot walks you through a short wizard:

1. **Bus number** — type the bus's short name (e.g. `306`). The bot checks it
   against the live TTC route list and asks again if no such bus exists.
2. **Stop** — type part of a stop name; the bot searches the TTC stop list and
   shows matches as buttons. (A stop is required because arrival times are
   per-stop — this step is what makes the reminder actually work.)
3. **Lead time** — pick how many minutes before arrival you want the reminder
   (5, 10, 15, 20, 25 or 30) from an inline keyboard.
4. **Start time** — pick the start of the daily active window. Times are shown
   in 30-minute steps, five at a time, with ◀️ / ▶️ arrows to scroll.
5. **End time** — pick the end of the window the same way.
6. **Name** — give the reminder a friendly name.

### `/list`, `/remove`, `/cancel`

- `/list` — show all your reminders.
- `/remove` — pick a reminder to delete from an inline keyboard.
- `/cancel` — abort an in-progress `/add` wizard.

### The notifier

A goroutine ticks every minute (and wakes instantly when a reminder is added).
For each reminder whose daily window currently contains the Tbilisi-local time,
it queries the stop's arrival times, finds the soonest real-time arrival of the
tracked bus, and — if that is within the lead time — sends a message. A 12-minute
cooldown per reminder prevents minute-by-minute spam while a bus approaches.

## Configuration

Copy `.env.example` to `.env` and fill it in:

| Variable             | Required | Default       | Description                                   |
| -------------------- | -------- | ------------- | --------------------------------------------- |
| `TELEGRAM_BOT_TOKEN` | yes      | —             | Bot token from [@BotFather][botfather].       |
| `DATABASE_PATH`      | no       | `data/bot.db` | Path to the SQLite database file.             |
| `TTC_LOCALE`         | no       | `en`          | Response language: `en` or `ka`.              |

## Running locally

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN

mkdir -p data
go run .
```

## Running with Docker

The image is built in two stages, so the final image contains only the static
binary (and CA certificates), with no Go toolchain or source.

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN

docker compose up --build -d
docker compose logs -f
```

The SQLite database is stored on the `bot-data` named volume and survives
container restarts.

## Development

```bash
go vet ./...
go build ./...
```

[ttc]: https://transit.ttc.com.ge
[botfather]: https://t.me/BotFather
