.PHONY: build dev test clean

build:
	wails build -skipbindings

dev:
	wails dev

test:
	go test ./...

clean:
	rm -rf build/bin
