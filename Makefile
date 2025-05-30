default: test

test:
	go vet ./...
	go test -v -race ./... -coverprofile=coverage.out

updatedeps:
	go get -f -t -u ./...
	go get -f -u ./...

.PHONY: default test updatedeps
