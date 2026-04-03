.PHONY: build test clean build-ts build-go build-rust test-ts test-go test-rust

build: build-ts build-go build-rust

test: test-ts test-go test-rust

build-ts:
	cd reverse-proxy-ts && npm ci && npm run build

build-go:
	cd reverse-proxy-go && go build ./...

build-rust:
	cd reverse-proxy-rust && cargo build --release

test-ts:
	cd reverse-proxy-ts && npm test

test-go:
	cd reverse-proxy-go && go test -race ./...

test-rust:
	cd reverse-proxy-rust && cargo test

run-ts:
	cd reverse-proxy-ts && npm start

run-go:
	cd reverse-proxy-go && go run ./cmd/server

run-rust:
	cd reverse-proxy-rust && cargo run

clean:
	cd reverse-proxy-ts && rm -rf dist node_modules 2>/dev/null || true
	cd reverse-proxy-go && go clean 2>/dev/null || true
	cd reverse-proxy-rust && cargo clean 2>/dev/null || true
