-include .env.local
-include .env
export

DOCSAF_BIN ?= ./bin/docsaf
ANTFLY_REPO ?= ../antfly
CORPUS_DIR ?= sample-corpus
DOCSAF_TABLE ?= docsaf
DOCSAF_OUTPUT ?= artifacts/docsaf-source-documents.json
INLINE_CONTENT ?= 1
BASE_URL ?=
PORT ?= 8766

.PHONY: help build-docsaf prepare sync query-smoke proxy web clean

help:
	@echo "DocsAF Cloud template targets:"
	@echo "  make build-docsaf  - build docsaf from ANTFLY_REPO/go/pkg/docsaf"
	@echo "  make prepare       - write source-document rows to $${DOCSAF_OUTPUT:-artifacts/docsaf-source-documents.json}"
	@echo "  make sync          - sync corpus to Antfly Cloud table"
	@echo "  make query-smoke   - run a small query against the table"
	@echo "  make proxy         - run local API-key proxy on PORT=$${PORT:-8766}"
	@echo "  make web           - serve local static search UI"
	@echo "Variables: DOCSAF_BIN ANTFLY_REPO CORPUS_DIR DOCSAF_TABLE INLINE_CONTENT BASE_URL ANTFLY_URL ANTFLYDB_API_KEY"

build-docsaf:
	@mkdir -p bin
	@test -d "$(ANTFLY_REPO)/go/pkg/docsaf" || (echo "Set ANTFLY_REPO to a checkout of github.com/antflydb/antfly, or set DOCSAF_BIN to an existing docsaf binary" && exit 1)
	cd $(ANTFLY_REPO)/go/pkg/docsaf && GOWORK=off go build -o $(CURDIR)/bin/docsaf ./cmd/docsaf

prepare:
	@mkdir -p $$(dirname "$(DOCSAF_OUTPUT)")
	$(DOCSAF_BIN) prepare --dir "$(CORPUS_DIR)" $(if $(filter 1 true yes,$(INLINE_CONTENT)),--inline-content,--base-url "$(BASE_URL)") --output "$(DOCSAF_OUTPUT)"

sync:
	@test -n "$(ANTFLY_URL)" || (echo "ANTFLY_URL is required" && exit 1)
	@test -n "$(ANTFLYDB_API_KEY)" || (echo "ANTFLYDB_API_KEY is required" && exit 1)
	$(DOCSAF_BIN) sync --url "$(ANTFLY_URL)" --token "$(ANTFLYDB_API_KEY)" --table "$(DOCSAF_TABLE)" --create-table --dir "$(CORPUS_DIR)" $(if $(filter 1 true yes,$(INLINE_CONTENT)),--inline-content,--base-url "$(BASE_URL)")

query-smoke:
	python3 scripts/query_smoke.py

proxy:
	PORT=$(PORT) python3 scripts/local_proxy.py

web:
	python3 -m http.server 8770 --directory web

clean:
	rm -rf artifacts bin
