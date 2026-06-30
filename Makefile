.PHONY: build test vet check fmt clean install

build:
	go build -o respawn .

test:
	go test ./...

vet:
	go vet ./...

check: vet test

fmt:
	gofmt -w .

install:
	go install .

clean:
	rm -f respawn
	rm -rf dist
