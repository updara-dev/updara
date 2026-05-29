.PHONY: setup dev build tidy build-agent-amd64 build-agent-arm64

# Run once after cloning to initialize Go modules and npm deps
setup:
	cd server && go mod tidy
	cd agent && go mod tidy
	cd frontend && npm install

# Start the full stack in dev mode (vite hot reload for frontend)
dev:
	docker compose up --build

# Build all images without starting
build:
	docker compose build

# Update go.sum files after changing go.mod
tidy:
	cd server && go mod tidy
	cd agent && go mod tidy

# Build agent binary for deployment (uses Docker, no local Go needed)
build-agent-amd64:
	docker run --rm -v "$(PWD)/agent:/app" -w /app golang:1.22-alpine \
		sh -c "go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o updara-agent-amd64 ."
	@echo "Binary ready: agent/updara-agent-amd64"

build-agent-arm64:
	docker run --rm -v "$(PWD)/agent:/app" -w /app golang:1.22-alpine \
		sh -c "go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o updara-agent-arm64 ."
	@echo "Binary ready: agent/updara-agent-arm64"
