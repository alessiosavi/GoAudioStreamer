.PHONY: build test test-short lint clean

build:
	CGO_ENABLED=1 go build -o bin/voxlink ./cmd/voxlink

test:
	CGO_ENABLED=1 go test -race -count=1 ./...

test-short:
	CGO_ENABLED=1 go test -short -race ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
