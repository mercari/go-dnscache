PACKAGES = $(shell go list ./...)
GO_TEST ?= go test
GO_TEST_FUNC ?= .

all: test

test:
	@${GO_TEST} -v -race -run=${GO_TEST_FUNC} $(PACKAGES)

vet:
	@go vet $(PACKAGES)

fmt:
	go fmt ./...

coverage:
	@${GO_TEST} -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...

reviewdog:
	@go get -u github.com/haya14busa/reviewdog/cmd/reviewdog
	reviewdog -reporter="github-pr-review"

changelog:
	@go get -u github.com/git-chglog/git-chglog/cmd/git-chglog
	git-chglog --output CHANGELOG.md

.PHONY: all test vet cover
