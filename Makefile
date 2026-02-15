default: all

.PHONY: all
all: dep test build

GIT_VERSION=$(shell git describe --always --tags --dirty)
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DATE=$(shell date +'%d.%m.%y %H:%M')

LDFLAGS=-ldflags "-s -X main.version=$(GIT_VERSION) -X main.commit=$(GIT_COMMIT) -X 'main.date=$(GIT_DATE)'"

.PHONY: clean
clean:
	[ -d dist ] || mkdir dist
	rm -rf dist/* || true

.PHONY: dep
dep:
	go mod tidy

.PHONY: checkdep
checkdep:
	go list -u -f '{{if (and (not (or .Main .Indirect)) .Update)}}{{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}' -m all 2> /dev/null

.PHONE: update
update:
	rm go.sum; go get -u ./...

.PHONY: test
test:
	go test -v ./...

.PHONY: build
build: clean dep
	go build $(LDFLAGS) -o dist/ ./cmd/...

.PHONY: gox
gox: clean dep
	GOARM=5 gox --osarch="linux/amd64 darwin/arm64" -output "dist/{{.OS}}_{{.Arch}}/{{.Dir}}" $(LDFLAGS) ./cmd/...
