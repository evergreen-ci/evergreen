all: test lint
test:
	go test -v -race .

lint:
	gometalinter --deadline=10m --disable="gocyclo" .

.PHONY: test lint all
