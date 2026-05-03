# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

.PHONY: all build generate generate-proto clean run test cli build-cli docker-local docker-kea-local lint fmt robot-test clean-branches dev-vm dev-vm-sync

all: generate build

generate: generate-proto

generate-proto:
	PATH=$(PATH):$(HOME)/go/bin protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/bng.proto
	PATH=$(PATH):$(HOME)/go/bin protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/ha/ha.proto

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

TEST_PACKAGES := $(shell go list ./... | grep -v /pkg/vpp/)

test:
	go test $(TEST_PACKAGES) -count=1 -timeout 120s

test-report:
	@mkdir -p build/reports
	@which gotestsum > /dev/null 2>&1 || go install gotest.tools/gotestsum@latest
	gotestsum --format pkgname --junitfile build/reports/unit-tests.xml -- $(TEST_PACKAGES) -count=1 -timeout 120s

build-cli: generate
	go build -o bin/osvbngcli ./cmd/osvbngcli

docker-local:
	docker build -f docker/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t veesixnetworks/osvbng:local .

docker-kea-local:
	docker build -f docker/dev/Dockerfile.kea \
		-t veesixnetworks/kea:local docker/dev

lint:
	golangci-lint run

fmt:
	golangci-lint run --fix

robot-test:
	./tests/rf-run.sh ./tests/$(suite) $(ROBOT_ARGS)

clean-branches:
	git fetch --prune origin
	git branch --format='%(refname:short)' | grep -v '^main$$' | while read branch; do \
		git show-ref --verify --quiet "refs/remotes/origin/$$branch" || git branch -D "$$branch"; \
	done

dev-vm:
	@scripts/dev/dev-vm.sh

dev-vm-sync:
	@scripts/dev/sync-vm.sh

dev-vm-provision:
	@scripts/dev/reprovision-vm.sh

.DEFAULT_GOAL := all
