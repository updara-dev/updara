FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /updara-server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /updara-server /updara-server
EXPOSE 8080
ENTRYPOINT ["/updara-server"]
