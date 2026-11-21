.DEFAULT_GOAL := install

.PHONY: install fmt test

install:
	go install ./cmd/zen

fmt:
	gofmt -w cmd

test:
	go vet ./...
	go test ./... -count=1
