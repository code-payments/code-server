all: generate

generate:
	docker run --rm -v $(PWD)/proto:/proto -v $(PWD)/gen:/genproto code-protobuf-api-builder-go

.PHONY: all generate