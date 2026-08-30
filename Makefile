.PHONY: build test vet run dry tidy clean

build:
	go build -o approval-scout .

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# Run against the real API using variables from .env
run:
	set -a; [ -f .env ] && . ./.env; set +a; go run .

# Same as run but forces DRY_RUN (prints instead of emailing)
dry:
	set -a; [ -f .env ] && . ./.env; set +a; DRY_RUN=true go run .

clean:
	rm -f approval-scout
