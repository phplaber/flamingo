VERSION=$(shell git describe --tags --always)

.PHONY: build_all
build_all:
	rm -rf bin && mkdir bin bin/linux-amd64 bin/darwin-amd64 bin/windows-amd64 \
	&& GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/darwin-amd64/ ./cmd/flamingo \
	&& GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/linux-amd64/ ./cmd/flamingo \
	&& GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/windows-amd64/ ./cmd/flamingo
