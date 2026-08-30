VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build run test test-unix fmt vet lint check

build:
	go build -ldflags '$(LDFLAGS)' -o beacon ./cmd/beacon

run:
	go run ./cmd/beacon

test:
	go test ./...

test-unix:
	go test -tags unix ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test ./...
