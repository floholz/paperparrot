.PHONY: dev ui build test docker shots clean

# terminal 1: make dev   (Go API on :8072)   terminal 2: cd ui && npm run dev (Vite on :5173, proxies /api)
dev:
	go run . serve --dir=./pb_data

ui:
	cd ui && npm ci && npm run build

# go:embed needs ui/dist to exist; `make ui` first (or once: mkdir -p ui/dist && touch ui/dist/index.html)
build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o paperparrot .

test:
	go vet ./... && go test ./...
	cd ui && npx svelte-check --tsconfig ./tsconfig.app.json --threshold error

docker:
	docker build -t paperparrot .

# screenshots of the running app: make shots URL=http://127.0.0.1:8072 EMAIL=... PASS=... OUT=./shots
shots:
	mkdir -p $(OUT) && go run ./tools/uishot $(URL) $(EMAIL) $(PASS) $(OUT) '#/documents' '#/fragments' '#/fonts'

clean:
	rm -rf paperparrot ui/dist
