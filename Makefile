.PHONY: all build test lint clean bench fmt vet docker-build docker-run config-init install release-dry help

all: build

build:
	go build -o distill .

test:
	go test ./...

lint: 
	golangci-lint run

bench:
	go test -bench=. -benchmem ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f distill

docker-build:
	docker build -t distill .

docker-run:
	docker run --rm -p 8080:8080 distill

config-init:
	./distill config init

install:
	go install

release-dry:
	goreleaser --snapshot --clean

help:
	@echo "Available targets:"
	@echo "  build:		- Build the distill binary"
	@echo "  test:			- Run all tests"
	@echo "  lint:			- Run golangci-lint"
	@echo "  bench:		- Run benchmarks"
	@echo "  fmt:			- Format Go source files"
	@echo "  vet:			- Run go vet"
	@echo "  clean:		- Remove the distill binary"
	@echo "  docker-build:		- Build the Docker image"
	@echo "  docker-run:		- Run the Docker container and remove it after exit"
	@echo "  config-init:		- Initialize the config file"
	@echo "  install:		- Install the binary"
	@echo "  release-dry:		- Dry run the release process"
	@echo "  help:			- Show this help message"