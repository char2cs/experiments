.PHONY: build test e2e e2e-real e2e-daemon fmt

build:
	CGO_ENABLED=0 go build -o bin/mcserver ./cmd/mcserver

test:
	go test -race -cover ./...

# Runs the manifest's own commands against the real server.
e2e:
	go test -race -tags e2e -run TestProcess ./e2e/

# Runs the minecraft arrow's commands against Mojang's real server (needs java, network).
e2e-real:
	E2E_REAL=1 go test -tags e2e -run TestMinecraft -v ./e2e/

# Drives a real quiver.core daemon, including the real minecraft + java install.
# QUIVER_BIN must point at a built quiver.
e2e-daemon:
	@test -n "$(QUIVER_BIN)" || (echo "set QUIVER_BIN=/path/to/quiver" && exit 1)
	E2E_REAL=1 QUIVER_BIN=$(QUIVER_BIN) go test -tags e2e -run TestQuiver -v ./e2e/

fmt:
	gofmt -w .
