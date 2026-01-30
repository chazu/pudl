# Makefile for pudl

BINARY_NAME := pudl
INSTALL_PATH := /usr/local/bin

# Build flags
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

.PHONY: all build install uninstall clean test

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

install: build
	install -m 755 $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)

uninstall:
	rm -f $(INSTALL_PATH)/$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	$(GO) clean

test:
	$(GO) test ./...

