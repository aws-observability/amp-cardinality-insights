.PHONY: build

build-CountMetricsFunction:
	GOOS=linux CGO_ENABLED=0 go build -o bootstrap
	cp ./bootstrap $(ARTIFACTS_DIR)/.
