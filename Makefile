BINARY := bin/memoryosd

.PHONY: test vet race build verify

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

build:
	mkdir -p bin
	go build -trimpath -ldflags='-s -w' -o $(BINARY) ./cmd/memoryosd

verify: test vet race build
	$(BINARY) version
