.PHONY: build build-windows build-linux run clean test fmt vet install help

# Binary name
BINARY_NAME=connectx
BINARY_WINDOWS=$(BINARY_NAME).exe
BINARY_LINUX=$(BINARY_NAME)

# Build for current platform
build:
	go build -o $(BINARY_WINDOWS) .

# Build for Windows
build-windows:
	SET GOOS=windows& SET GOARCH=amd64& go build -o $(BINARY_WINDOWS) .

# Build for Linux
build-linux:
	SET GOOS=linux& SET GOARCH=amd64& go build -o $(BINARY_LINUX) .

# Build for both platforms
build-all: build-windows build-linux

# Run the application
run:
	@go run .

# Build and run
build-run: build
	.\$(BINARY_WINDOWS)

# Clean build artifacts
clean:
	go clean
	if exist $(BINARY_WINDOWS) del $(BINARY_WINDOWS)
	if exist $(BINARY_LINUX) del $(BINARY_LINUX)

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Install dependencies
install:
	go mod download
	go mod tidy

# Run all checks
check: fmt vet test

# Show help
help:
	@echo Available targets:
	@echo   build         - Build for current platform (Windows)
	@echo   build-windows - Build for Windows (amd64)
	@echo   build-linux   - Build for Linux (amd64)
	@echo   build-all     - Build for both Windows and Linux
	@echo   run           - Run the application
	@echo   build-run     - Build and run the application
	@echo   clean         - Remove build artifacts
	@echo   test          - Run tests
	@echo   fmt           - Format code
	@echo   vet           - Run go vet
	@echo   install       - Install dependencies
	@echo   check         - Run fmt, vet, and test
	@echo   help          - Show this help message
