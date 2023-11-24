all: test

test:
	@go test -cover ./...

.PHONY: all test
