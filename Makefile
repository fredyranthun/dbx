.PHONY: fmt test check

fmt:
	gofmt -w .

test:
	go test ./...

check: fmt test
