.DEFAULT_GOAL := install

.PHONY: install fmt test

install:
	go install ./cmd/calmpane

fmt:
	gofmt -w claudecode cmd internal

test:
	go vet ./...
	go test ./... -count=1
