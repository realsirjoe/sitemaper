APP := sitemaper
BIN := ./$(APP)
GOCACHE ?= /tmp/gocache
GOENV := GOWORK=off GOCACHE=$(GOCACHE)

.PHONY: build test test-integration fmt clean

build:
	mkdir -p $(GOCACHE)
	$(GOENV) go build -o $(BIN) ./cmd/$(APP)

test:
	mkdir -p $(GOCACHE)
	$(GOENV) go test ./...

fmt:
	$(GOENV) gofmt -w $$(find . -name '*.go' -type f)

clean:
	rm -f $(BIN)
