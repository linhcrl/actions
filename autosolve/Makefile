.PHONY: build test clean

# Local dev binary
build:
	go build -o autosolve ./cmd/autosolve

test:
	go test ./... -count=1

clean:
	rm -f autosolve
