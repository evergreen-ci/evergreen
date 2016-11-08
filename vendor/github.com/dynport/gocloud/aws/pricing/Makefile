test: build
	go test -v

build: assets
	go get .

assets: install_goassets
	rm -f ./assets.go
	goassets assets

install_goassets:
	which goassets || go get github.com/dynport/dgtk/goassets
