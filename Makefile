.PHONY: all
all: test

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
