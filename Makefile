.PHONY: all build generate generate-proto clean run test cli build-cli docker-local

all: generate build

generate: generate-proto

generate-proto:
	PATH=$(PATH):$(HOME)/go/bin protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/bng.proto

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/veesix-networks/osvbng/pkg/version.Version=$(VERSION) \
           -X github.com/veesix-networks/osvbng/pkg/version.Commit=$(COMMIT) \
           -X github.com/veesix-networks/osvbng/pkg/version.Date=$(DATE)

build: generate
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/osvbngd ./cmd/osvbngd
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/osvbngcli ./cmd/osvbngcli

clean:
	rm -rf bin/
	rm -f api/proto/*.pb.go

run: build
	sudo ./bin/osvbngd -config configs/config.yaml

deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go mod download
	go mod tidy

test:
	go test -v ./...

build-cli: generate
	go build -o bin/osvbngcli ./cmd/osvbngcli

docker-local:
	docker build -f docker/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t veesixnetworks/osvbng:local .

.DEFAULT_GOAL := all
