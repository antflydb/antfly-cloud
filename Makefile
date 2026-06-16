.PHONY: test test-go test-ts generate generate-ts

test: test-go test-ts

test-go:
	cd go && GOWORK=off GOCACHE=/tmp/antfly-cloud-go-build-cache go test ./...

test-ts:
	cd ts && pnpm test

generate: generate-ts

generate-ts:
	cd ts && pnpm --filter @antflydb/antfly-cloud-sdk generate
