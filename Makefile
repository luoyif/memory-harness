BINARY := bin/memoryosd
VERSION ?= 2.2.0
LINUX_RELEASE_DIR ?= build/release/linux

.PHONY: test vet race build build-linux verify

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

build:
	mkdir -p bin
	go build -trimpath -ldflags='-s -w' -o $(BINARY) ./cmd/memoryosd

build-linux:
	./scripts/build-linux-release.sh $(VERSION) $(LINUX_RELEASE_DIR)

verify: test vet race build
	$(BINARY) version
