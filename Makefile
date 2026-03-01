.PHONY: build install clean test

build:
	go build -tags fts5 -o qqmd .

install:
	go install -tags fts5 .

clean:
	rm -f qqmd

test:
	go test -tags fts5 ./...
