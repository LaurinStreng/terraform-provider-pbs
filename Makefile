NAME=terraform-provider-pbs
VERSION=1.0.1

default: build

.PHONY: build
build:
	go build -o ${NAME} .

.PHONY: install
install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/laurinstreng/pbs/${VERSION}/darwin_amd64
	cp ${NAME} ~/.terraform.d/plugins/registry.terraform.io/laurinstreng/pbs/${VERSION}/darwin_amd64/

.PHONY: test
test:
	go test ./...

# Acceptance tests: see ./testacc --help and CLAUDE.md
.PHONY: testacc
testacc:
	./testacc

.PHONY: testacc-env
testacc-env:
	docker compose up -d
	@test -f testacc.env || cp testacc.env.example testacc.env

.PHONY: lint
lint:
	golangci-lint run

.PHONY: fmt
fmt:
	gofmt -s -w .
	go mod tidy

.PHONY: clean
clean:
	rm -f ${NAME}

.PHONY: docs
docs:
	go tool tfplugindocs generate --provider-name pbs

.PHONY: release-test
release-test:
	goreleaser release --snapshot --clean --skip=publish

.PHONY: release
release:
	@echo "To create a release:"
	@echo "  1. Ensure all changes are committed"
	@echo "  2. Create and push a tag: git tag -a v1.0.1 -m 'Release v1.0.1'"
	@echo "  3. Push the tag: git push origin v1.0.1"
	@echo "  4. GitHub Action will automatically build and publish"


.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Build the provider binary"
	@echo "  install       - Build and install the provider locally"
	@echo "  test          - Run unit tests"
	@echo "  testacc       - Run acceptance tests (./testacc, needs testacc.env)"
	@echo "  testacc-env   - Start local PBS test stack via docker compose"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  clean         - Remove built binaries"
	@echo "  docs          - Generate documentation"
	@echo "  release-test  - Test release build locally (requires goreleaser)"
	@echo "  release       - Show release instructions"
	@echo "  help          - Show this help message"