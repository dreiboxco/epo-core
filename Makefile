.PHONY: build test lint fmt vet tidy clean help

BIN_DIR := bin
BIN_NAME := epo
PKG := ./...

help:
	@echo "Targets:"
	@echo "  build   Build the epo binary into $(BIN_DIR)/$(BIN_NAME)"
	@echo "  test    Run unit tests"
	@echo "  lint    Run go vet (staticcheck/golangci-lint can be added later)"
	@echo "  fmt     Run gofmt -s -w on all Go files"
	@echo "  tidy    Run go mod tidy"
	@echo "  clean   Remove build artifacts"

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BIN_NAME) ./cmd/epo

test:
	go test -race -count=1 $(PKG)

lint: vet

vet:
	go vet $(PKG)

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
