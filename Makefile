# winfit is a Windows-only tool. On Windows, MSYS2/Git sh.exe is blocked by
# endpoint protection (fork errors), so force cmd.exe for recipes and
# $(shell) — never sh/bash.
ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
.SHELLFLAGS := /c
NULLDEV := NUL
else
NULLDEV := /dev/null
endif

EXECUTABLE?=winfit
# Package to build (the CLI spans main.go + run.go + flags.go, so build the
# package dir, not a single file).
MAIN?=.

# Fall back to "dev" when git cannot describe the tree (fresh scaffold, or a
# blocked git/shim invocation — build must still succeed).
VERSION=$(shell git describe --tags --always --dirty 2>$(NULLDEV))
ifeq ($(strip $(VERSION)),)
VERSION=dev
endif

BUILDFLAGS=-trimpath -ldflags="-s -w -X main.version=$(VERSION)"

# Host build needs no GOOS/GOARCH override — go defaults to the host, which
# keeps sh-style `VAR=value cmd` syntax out of the recipe. On Windows the
# output gets .exe; elsewhere it stays extension-free.
ifeq ($(OS),Windows_NT)
default:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/$(EXECUTABLE).exe $(MAIN)
else
default:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/$(EXECUTABLE) $(MAIN)
endif

# Cross-compiled Windows binaries (also how you produce the release binary
# from a non-Windows host, e.g. CI).
windows:
ifeq ($(OS),Windows_NT)
	set GOOS=windows&& set GOARCH=amd64&& set CGO_ENABLED=0&& go build $(BUILDFLAGS) -o build/$(EXECUTABLE)_windows_amd64.exe $(MAIN)
else
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(BUILDFLAGS) -o build/$(EXECUTABLE)_windows_amd64.exe $(MAIN)
endif

ifeq ($(OS),Windows_NT)
clean: ## Remove build artifacts
	-del /f /q bin\$(EXECUTABLE).exe 2>NUL
	-rd /s /q build 2>NUL
else
clean: ## Remove build artifacts
	rm -f bin/$(EXECUTABLE)
	rm -rf build
endif

test: ## Run tests
	go test ./...

fmt: ## Format the code
	gofmt -l -w .

run: ## Run the application
	go run $(MAIN)

help: ## Display available commands
	@echo default   Build for the host into bin/
	@echo windows   Cross-build the windows_amd64 binary into build/
	@echo test      go test ./...
	@echo run       go run the application
	@echo fmt       gofmt -l -w .
	@echo clean     Remove build artifacts

.PHONY: default windows clean test fmt run help
