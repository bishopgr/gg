.PHONY: all build bump test 

all: test build bump

bump:
	@echo "Bumping version..."
	@pwsh .\scripts\build.ps1

build:
	go build -o bin/gg.exe cmd/gg/main.go

test: 
	go test -v -cover ./...
