all: generate

generate:
	docker run --platform linux/amd64 -v $(shell pwd)/:/proto -v $(shell pwd)/:/genproto kinecosystem/protoc-gen-go

.PHONY: all generate
