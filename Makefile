GO ?= go
.PHONY: build controller-mac migrate

build:
	mkdir -p bin
	cd server && $(GO) build -trimpath -o ../bin/gameserver ./cmd/gameserver
	cd server && $(GO) build -trimpath -o ../bin/dispatch ./cmd/dispatch
	cd server && $(GO) build -trimpath -o ../bin/migrate ./cmd/migrate

controller-mac:
	./controller/build-app.sh

migrate:
	@test -n "$${DATABASE_URL:-}" || (echo '请显式设置 DATABASE_URL' >&2; exit 2)
	cd server && $(GO) run ./cmd/migrate -dsn "$$DATABASE_URL" -dir migrations
