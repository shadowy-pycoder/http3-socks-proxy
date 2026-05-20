.PHONY: all
all: build

.PHONY: build
build:
	CGO_ENABLED=0 go build -trimpath -o ./bin/proxy ./cmd/proxy/*.go
	CGO_ENABLED=0 go build -trimpath -o ./bin/client ./cmd/client/*.go

