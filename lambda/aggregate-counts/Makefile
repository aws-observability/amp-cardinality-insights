.PHONY: build

build-AggregateCountFunction:
	GOOS=linux CGO_ENABLED=0 go build -o bootstrap
	cp ./bootstrap $(ARTIFACTS_DIR)/.
