.PHONY: run lint test

default: run

# make only=nextcloud-data dryrun=1
run:
	go run ./cmd/backpull $(if $(only),-only $(only)) $(if $(dryrun),-dry-run)

test:
	go test -count=1 ./...

lint:
	golangci-lint run
