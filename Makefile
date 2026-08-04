.PHONY: dev install migrate test verify

install:
	go mod download
	pnpm --dir web install --frozen-lockfile

migrate:
	go run ./cmd/migrate

dev:
	./scripts/dev.sh

test:
	go test ./...
	pnpm --dir web test

verify:
	test -z "$$(gofmt -l cmd db internal)"
	go test ./...
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
	pnpm --dir web typecheck
	pnpm --dir web lint
	pnpm --dir web test
	pnpm --dir web build
	pnpm --dir web test:e2e
