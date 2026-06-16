.PHONY: test test-go test-ts generate generate-ts

test: test-go test-ts

test-go:
	cd go && GOWORK=off go test ./...

test-ts:
	cd ts && pnpm install --frozen-lockfile && pnpm test

generate: generate-ts

generate-ts:
	cd ts && pnpm --filter @antflydb/antfly-cloud-sdk generate
