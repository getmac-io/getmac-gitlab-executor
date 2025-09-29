GO    	:= go
GOFMT 	:= gofmt

export CGO_LDFLAGS := "-Wl,-no_warn_duplicate_libraries"

.PHONY: build
build:
		@ $(GO) build -o ./dist/getmac-gitlab-executor ./cmd/getmac-gitlab-executor

.PHONY: fmt
fmt:
		@ $(GOFMT) -w -s .

.PHONY: test
test:
		@ $(GO) test -v ./...
