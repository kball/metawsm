.PHONY: dev-backend dev-frontend frontend-check generate-frontend build test

UI_DIR ?= ui
DEV_API_ADDR ?= :3001
DEV_UI_PORT ?= 3000

dev-backend:
	go run ./cmd/metawsm web --addr "$(DEV_API_ADDR)"

dev-frontend:
	npm --prefix $(UI_DIR) run dev -- --host --port "$(DEV_UI_PORT)"

frontend-check:
	npm --prefix $(UI_DIR) run check

generate-frontend:
	go generate ./internal/web

build:
	go generate ./internal/web && go build -tags embed ./...

test:
	go test ./... -count=1
