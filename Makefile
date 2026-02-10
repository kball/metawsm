.PHONY: dev-backend dev-frontend dev-all frontend-check ui-install generate-frontend prepare-serve serve-all build test

UI_DIR ?= ui
DEV_API_ADDR ?= :3001
DEV_UI_PORT ?= 3000
DEV_DB ?= .metawsm/metawsm.db
SERVE_ADDR ?= :3001
SERVE_DB ?= .metawsm/metawsm.db

dev-backend:
	go run ./cmd/metawsm serve --addr "$(DEV_API_ADDR)"

dev-frontend:
	npm --prefix $(UI_DIR) run dev -- --host --port "$(DEV_UI_PORT)"

dev-all: ui-install
	@set -e; \
	echo "Starting metawsm serve on $(DEV_API_ADDR) and Vite on $(DEV_UI_PORT)"; \
	go run ./cmd/metawsm serve --addr "$(DEV_API_ADDR)" --db "$(DEV_DB)" & \
	backend_pid=$$!; \
	trap 'kill $$backend_pid' EXIT INT TERM; \
	npm --prefix $(UI_DIR) run dev -- --host --port "$(DEV_UI_PORT)"

frontend-check:
	npm --prefix $(UI_DIR) run check

ui-install:
	npm --prefix $(UI_DIR) install

generate-frontend:
	go generate ./internal/web

prepare-serve: ui-install
	go generate ./internal/web

serve-all: prepare-serve
	go run ./cmd/metawsm serve --addr "$(SERVE_ADDR)" --db "$(SERVE_DB)"

build: prepare-serve
	go build -tags embed ./...

test:
	go test ./... -count=1
