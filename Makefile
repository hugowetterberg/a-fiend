.PHONY: build xbuild build-windows build-linux

build:
	GOOS=darwin GOARCH=amd64 go build -o bin/a-fiend

clean:
	rm -fr bin

xbuild: clean build build-windows build-linux
	zip bin/a-fiend.darwin-amd64.zip bin/a-fiend

build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/linux/a-fiend
	zip bin/a-fiend.linux-amd64.zip bin/linux/a-fiend

build-windows:
	GOOS=windows GOARCH=amd64 go build -o bin/windows/a-fiend
	zip bin/a-fiend.windows-amd64.zip bin/windows/a-fiend
