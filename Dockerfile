# ---- build stage ----
FROM golang:1.25-alpine AS build

WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static binary (modernc.org/sqlite is pure Go, no CGO needed).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bot .

# ---- final stage ----
# distroless/static contains only CA certificates and the binary we copy in.
FROM gcr.io/distroless/static-debian12

WORKDIR /data
COPY --from=build /bot /bot

ENV DATABASE_PATH=/data/bot.db

ENTRYPOINT ["/bot"]
