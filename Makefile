VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test test-unix fmt vet check

build:
	go build -ldflags '$(LDFLAGS)' -o beacon ./cmd/beacon

test:
	go test ./...

test-unix:
	go test -tags unix ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test ./...
