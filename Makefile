.PHONY: all build web clean run dev test build-mac-app

APP_NAME = codex-go
WEB_DIR = web

all: web build

web:
	cd $(WEB_DIR) && npm install && npm run build

build:
	go build -ldflags "-H windowsgui -s -w" -o $(APP_NAME) ./cmd/codex-go/

build-linux:
	cd $(WEB_DIR) && npm run build
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $(APP_NAME)-linux ./cmd/codex-go/

build-mac:
	cd $(WEB_DIR) && npm run build
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o $(APP_NAME)-mac ./cmd/codex-go/

build-mac-app:
	cd $(WEB_DIR) && npm run build
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o $(APP_NAME)-darwin-arm64 ./cmd/codex-go/
	@mkdir -p "$(APP_NAME).app/Contents/MacOS"
	@mkdir -p "$(APP_NAME).app/Contents/Resources"
	@cp $(APP_NAME)-darwin-arm64 "$(APP_NAME).app/Contents/MacOS/codex-go"
	@cp cmd/codex-go/Info.plist "$(APP_NAME).app/Contents/"
	@cp cmd/codex-go/codex-go.icns "$(APP_NAME).app/Contents/Resources/codex-go.icns"

build-win:
	cd $(WEB_DIR) && npm run build
	GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o $(APP_NAME).exe ./cmd/codex-go/

run:
	go run -ldflags "-H windowsgui -s -w" ./cmd/codex-go/

dev:
	cd $(WEB_DIR) && npm run dev &
	go run -ldflags "-H windowsgui -s -w" ./cmd/codex-go/

clean:
	rm -rf cmd/codex-go/web-dist $(APP_NAME) $(APP_NAME)-linux $(APP_NAME)-mac $(APP_NAME).exe $(APP_NAME).app $(APP_NAME)-darwin-*

test:
	go test ./... -v -count=1 -timeout 60s
