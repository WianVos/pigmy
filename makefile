RELEASE_DIR=./release


.PHONY: build_darwin
build_darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -installsuffix cgo -o ${RELEASE_DIR}/pigmy .

