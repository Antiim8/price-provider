SHELL := /usr/bin/env bash

APP := price-provider
GO ?= go

.PHONY: all build run server fetch steamdt-dump test tidy fmt vet

all: build

build:
	$(GO) build ./...

server:
	$(GO) run ./cmd/server

fetch:
	$(GO) run ./cmd/fetch

steamdt-dump:
	$(GO) run ./cmd/steamdt_dump \
		--symbols-file pricempire_all_prices.json \
		--out steamdt_all_prices.json \
		--batch 50 --concurrency 4 --timeout 20 --retries 3 --rpm 0

run: server

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...
