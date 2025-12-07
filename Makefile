build:
	@go build -o bin/fs ./cmd/node

run: build
	@./bin/fs

# default test (verbose)
test:
	@go test -v ./...

# pretty, colorful tests
test-pretty:
	@richgo test ./...
