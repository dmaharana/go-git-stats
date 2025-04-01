# Makefile for building the Go program gitstat for Windows and Linux

# Go compiler
GO := go

# Build flags
LDFLAGS := -ldflags "-s -w"

# Output binary names
WINDOWS_BINARY := gitstat.exe
LINUX_BINARY := gitstat
BIN_DIR := bin

# Default target
all: windows linux

# Build for Windows
windows:
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(WINDOWS_BINARY) main.go

# Build for Linux
linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(LINUX_BINARY) main.go

# Clean up build artifacts
clean:
	rm -f $(BIN_DIR)/$(WINDOWS_BINARY) $(BIN_DIR)/$(LINUX_BINARY)
