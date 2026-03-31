.PHONY: build run test vet clean

build:
	CGO_ENABLED=0 go build -o trough ./cmd/trough/

run: build
	TROUGH_ADMIN_KEY=dev-admin DATA_DIR=./data ./trough

test:
	go test ./... -count=1 -timeout 60s

vet:
	go vet ./...

clean:
	rm -f trough
