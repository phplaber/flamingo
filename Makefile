version=$(shell git describe --tags --always)

.PHONY: build_all
build_all:
	rm -rf bin && mkdir -p bin/linux-amd64 bin/darwin-amd64 bin/windows-amd64 \
	&& GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$(version)" -o ./bin/darwin-amd64/ \
	&& GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(version)" -o ./bin/linux-amd64/ \
	&& GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$(version)" -o ./bin/windows-amd64/
