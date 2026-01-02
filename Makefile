.PHONY: all build generate generate-proto clean run test cli build-cli

all: generate build

generate: generate-proto

generate-proto:
	PATH=$(PATH):$(HOME)/go/bin protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/bng.proto

build: generate
	go build -o bin/osvbngd ./cmd/osvbngd
	go build -o bin/osvbngcli ./cmd/osvbngcli

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

.DEFAULT_GOAL := all
