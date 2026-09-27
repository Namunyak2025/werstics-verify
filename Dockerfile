FROM node:22-alpine AS web-build

WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build


FROM golang:1.25-alpine AS go-build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY backend ./backend

RUN CGO_ENABLED=0 GOOS=linux go build \
    -o /app/werstics-verify \
    ./backend/cmd/server


FROM alpine:3.22

RUN apk add --no-cache \
    bash \
    postgresql-client \
    ca-certificates

WORKDIR /app

COPY --from=go-build /app/werstics-verify ./werstics-verify
COPY --from=web-build /app/web/dist ./web/dist
COPY db ./db
COPY scripts ./scripts
COPY docker-entrypoint.sh ./docker-entrypoint.sh

RUN chmod +x ./docker-entrypoint.sh ./scripts/migrate.sh

ENV WERSTICS_VERIFY_ADDR=:10000

EXPOSE 10000

CMD ["./docker-entrypoint.sh"]
