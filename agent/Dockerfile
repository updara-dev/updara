FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /updara-agent .

FROM alpine:3.20
# sqlite3 is needed by some connectors (e.g. pihole)
RUN apk add --no-cache bash sqlite
COPY --from=builder /updara-agent /updara-agent
ENTRYPOINT ["/updara-agent"]
