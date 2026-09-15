BINARY := kctl

.PHONY: build test lint fmt cover run clean check

build:
	go build -o $(BINARY) ./cmd/kctl

test:
	go test ./... -race -count=1

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

fmt:
	gofmt -l -w ./cmd ./internal

# What CI runs. Run this before pushing.
check: fmt
	go build ./...
	go vet ./...
	go test ./... -race -count=1

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY) coverage.out
