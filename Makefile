.PHONY: build proto clean install

# Binary names
BINARY_NAME=crabby

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Proto parameters
PROTOC=protoc
PROTO_DIR=internal/api
PROTO_FILES=$(PROTO_DIR)/messages.proto

# Build flags
LDFLAGS=-ldflags "-s -w"

all: proto build

build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/crabby

proto:
	$(PROTOC) --go_out=. --go_opt=paths=source_relative $(PROTO_FILES)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(PROTO_DIR)/*.pb.go

install: build
	mv $(BINARY_NAME) $(GOPATH)/bin/

deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Development helpers
run-daemon: build
	./$(BINARY_NAME) daemon

run-status: build
	./$(BINARY_NAME) status

run-chat: build
	./$(BINARY_NAME) chat
