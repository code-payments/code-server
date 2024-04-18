.PHONY: all
all: lint test

.PHONY: test
test:
	@go test -cover ./...


.PHONY: integration-test
integration-test:
	@go test -tags integration -cover -timeout=5m ./...


.PHONY: clean-integration-containers
clean-integration-containers:
	@echo Removing etcd containers...
	@docker ps | grep -E "etcd-test-[0-9a-z]{8}-[0-9]+" | awk '{print $$1}' | xargs docker rm -f 2>/dev/null || true
	@echo Removing etcd cluster networks...
	@docker network ls | grep -E "etcd-test-[0-9a-z]{8}-network" | awk '{print $$1}' | xargs docker network remove 2>/dev/null || true

.PHONY: lint
lint: tools.golangci-lint
	@golangci-lint --timeout=3m --config .golangci-lint.yaml run ./...

.PHONY: tools
tools: tools.golangci-lint

tools.golangci-lint: .build/markers/golangci-lint_installed
.build/markers/golangci-lint_installed:
	@command -v golangci-lint >/dev/null ; if [ $$? -ne 0 ]; then \
		CGO_ENABLED=0 go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.2; \
	fi
	@mkdir -p $(shell dirname $@) && touch "$@"
