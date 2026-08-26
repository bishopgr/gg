.PHONY: build all test meta bump


test: 
	go test -v ./...

build:
	go build -o bin/gg.exe cmd/gg/main.go

meta: meta
	
bump:
	@echo "Bumping version..."
	@pwsh .\scripts\build.ps1

all: test build meta bump