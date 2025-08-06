.PHONY: build

## Build wake cli
build:
	@go build  -o wake ./cmd/wake/main.go
