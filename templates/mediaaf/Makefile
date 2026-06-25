# MediaAF Makefile
#
# Prerequisites:
#   - Antfly Cloud account and instance
#   - antfly cloud logged in and able to show connection details
#   - Description provider chosen explicitly; prefer Antfly Cloud inference when available

# Load .env files if they exist
-include .env .env.local
export

# Antfly API URL
ANTFLY_URL ?= http://localhost:8080/api/v1

# Ingest settings
INGEST_BATCH_SIZE ?= 50
INGEST_TABLE ?= mediaaf

# Describe settings
WORKERS ?= 20
DESCRIPTION_PROVIDER ?= antfly
DESCRIPTION_MODEL ?= gemini-2.5-flash
# Back-compat: MODEL overrides DESCRIPTION_MODEL if supplied by the caller.
MODEL ?= $(DESCRIPTION_MODEL)
DESCRIBE_OUTPUT_DIR := ingest/image-to-text/output
N ?= 100
MEDIA_BASE_URL ?=

# Sources
SOURCES_DIR := sources

.PHONY: help setup proxy describe-sample scrape upload upload-all describe review ingest status web web-remote web-install web-build test lint clean test-prompt mood-test mood

help:
	@echo "MediaAF - Available targets:"
	@echo ""
	@echo "  --- Pipeline ---"
	@echo "  scrape SRC=X           - Discover media items for source X (build manifest)"
	@echo "  upload SRC=X           - Upload source X media to R2"
	@echo "  upload-all             - Upload all sources to R2"
	@echo "  describe N=100         - Generate descriptions from R2 (single model)"
	@echo "  describe N=100 SRC=X   - Describe only source X (R2 prefix filter)"
	@echo "  describe DESCRIPTION_PROVIDER=X MODEL=Y - Generate descriptions (default provider: $(DESCRIPTION_PROVIDER), model: $(MODEL))"
	@echo "  describe-sample        - Print transform smoke-test guidance"
	@echo "  review                 - Regenerate review.md from description files"
	@echo "  ingest                 - Ingest latest describe output into Antfly"
	@echo "  status                 - Show pipeline status for all sources"
	@echo ""
	@echo "  --- Mood Classification ---"
	@echo "  mood-test              - Classify 50 moods for spot-checking"
	@echo "  mood                   - Full pipeline: classify all moods + patch Antfly"
	@echo ""
	@echo "  --- Dev ---"
	@echo "  test-prompt N=10       - Quick prompt iteration test (N media items from local TGIF TSV)"
	@echo "  setup                  - Install pipeline dependencies (Playwright, etc.)"
	@echo ""
	@echo "  --- Web ---"
	@echo "  proxy                  - Run local API-key proxy on :8765"
	@echo "  web                    - Start the web development server (local proxy)"
	@echo "  web-remote             - Start the web dev server against ANTFLY_PROXY_URL"
	@echo "  web-install            - Install web dependencies"
	@echo "  web-build              - Build web app for production"
	@echo "  test                   - Run web tests"
	@echo "  lint                   - Run linter"
	@echo "  clean                  - Remove build artifacts"

# ============================================================
# Pipeline
# ============================================================

describe-sample:
	@echo "MediaAF sample description depends on the selected inference provider."
	@echo "Preferred provider: DESCRIPTION_PROVIDER=antfly with ANTFLY_INFERENCE_URL set."
	@echo "Find the Cloud inference URL with: antfly cloud connection <instance> --json"
	@echo "For local media, implement/update a transform in transforms/ and run describe after staging media."
	@echo "Transform smoke example: python3 transforms/static_image.py --input <image> --output /tmp/mediaaf-package.json"

setup:
	uv run --with playwright python -m playwright install chromium

scrape:
	@test -n "$(SRC)" || (echo "Usage: make scrape SRC=<source_name>" && exit 1)
	@test -f "$(SOURCES_DIR)/$(SRC)/scrape.py" || (echo "Error: $(SOURCES_DIR)/$(SRC)/scrape.py not found" && exit 1)
	uv run $(SOURCES_DIR)/$(SRC)/scrape.py

upload:
	@test -n "$(SRC)" || (echo "Usage: make upload SRC=<source_name>" && exit 1)
	uv run ingest/save-to-r2/upload_r2.py --source $(SRC)

upload-all:
	uv run ingest/save-to-r2/upload_r2.py --all-sources

# Generate descriptions from R2 bucket (single model)
# Optional SRC= filters to a single source via R2 prefix
# DESCRIPTION_PROVIDER must be explicit. Supported values: antfly, genai, openrouter.
describe:
	@test -n "$(R2_BUCKET)" || (echo "Error: R2_BUCKET not set" && exit 1)
	@test -n "$(DESCRIPTION_PROVIDER)" || (echo "Error: DESCRIPTION_PROVIDER not set. Prefer antfly if available; otherwise ask the user." && exit 1)
	@test -n "$(MODEL)" || (echo "Error: DESCRIPTION_MODEL/MODEL not set" && exit 1)
	@if [ "$(DESCRIPTION_PROVIDER)" = "antfly" ] && [ -z "$(ANTFLY_INFERENCE_URL)" ]; then \
		echo "Error: ANTFLY_INFERENCE_URL not set for DESCRIPTION_PROVIDER=antfly."; \
		echo "Find it with: antfly cloud connection <instance> --json"; \
		echo "Use the antfly_inference_proxy_url value, then rerun."; \
		exit 1; \
	fi
	@if [ "$(DESCRIPTION_PROVIDER)" = "genai" ] && [ -z "$(GEMINI_API_KEY)$(GOOGLE_API_KEY)" ]; then \
		echo "Error: Gemini credentials not set. Set GEMINI_API_KEY/GOOGLE_API_KEY or ask the user which provider to use."; \
		exit 1; \
	fi
	@if [ "$(DESCRIPTION_PROVIDER)" = "openrouter" ] && [ -z "$(OPENROUTER_API_KEY)" ]; then \
		echo "Error: OPENROUTER_API_KEY not set. Ask the user before switching providers."; \
		exit 1; \
	fi
	@mkdir -p $(DESCRIBE_OUTPUT_DIR)
	$(eval DESCRIBE_SAFE_NAME := $(subst /,-,$(DESCRIPTION_PROVIDER)-$(MODEL)))
	uv run ingest/image-to-text/describe.py \
		--source r2 \
		--r2-bucket "$(R2_BUCKET)" \
		$(if $(SRC),--r2-prefix "sources/$(SRC)/") \
		--backend "$(DESCRIPTION_PROVIDER)" \
		--model "$(MODEL)" \
		--output "$(DESCRIBE_OUTPUT_DIR)/descriptions-$(DESCRIBE_SAFE_NAME).jsonl" \
		--prompt "ingest/image-to-text/prompt.txt" \
		--workers $(WORKERS) \
		--limit $(N) \
		--resume \
		$(if $(ONLY_UNPROCESSED_BY),--only-unprocessed-by "$(ONLY_UNPROCESSED_BY)")
	ln -sf descriptions-$(DESCRIBE_SAFE_NAME).jsonl $(DESCRIBE_OUTPUT_DIR)/descriptions-latest.jsonl
	@echo "Active provider/model: $(DESCRIPTION_PROVIDER)/$(MODEL)"
	$(MAKE) review

# Ingest description output into Antfly
ingest:
	@test -L "$(DESCRIBE_OUTPUT_DIR)/descriptions-latest.jsonl" || \
		(echo "Error: no descriptions-latest.jsonl symlink. Run 'make describe' first." && exit 1)
	@echo "Ingesting: $$(readlink $(DESCRIBE_OUTPUT_DIR)/descriptions-latest.jsonl)"
	uv run ingest/embed-text-descriptions/embed.py \
		--jsonl "$(DESCRIBE_OUTPUT_DIR)/descriptions-latest.jsonl" \
		--url "$(ANTFLY_URL)" \
		--table "$(INGEST_TABLE)" \
		--batch-size $(INGEST_BATCH_SIZE) \
		--media-base-url "$(MEDIA_BASE_URL)"

# Generate review markdown from all description files
review:
	uv run ingest/image-to-text/review.py \
		--input $(DESCRIBE_OUTPUT_DIR)/descriptions-*.jsonl \
		--output "$(DESCRIBE_OUTPUT_DIR)/review.md"

status:
	@echo "Source pipeline status:"
	@echo ""
	@for manifest in $(SOURCES_DIR)/*/manifest.json; do \
		if [ -f "$$manifest" ]; then \
			src=$$(basename $$(dirname "$$manifest")); \
			total=$$(python3 -c "import json; m=json.load(open('$$manifest')); print(len(m['items']))"); \
			uploaded=$$(python3 -c "import json; m=json.load(open('$$manifest')); print(sum(1 for i in m['items'] if i.get('r2_url')))"); \
			described=$$(python3 -c "import json; m=json.load(open('$$manifest')); print(sum(1 for i in m['items'] if i.get('described')))"); \
			echo "  $$src: $$total items, $$uploaded on R2, $$described described"; \
		fi \
	done
	@if [ ! -d "$(SOURCES_DIR)" ] || [ -z "$$(ls $(SOURCES_DIR)/*/manifest.json 2>/dev/null)" ]; then \
		echo "  (no sources scraped yet)"; \
	fi

# ============================================================
# Mood Classification
# ============================================================

MOOD_JSONL ?= ingest/image-to-text/output/descriptions-gemini-2.5-flash-lite.jsonl

mood-test:
	uv run ingest/classify-moods/classify.py --jsonl $(MOOD_JSONL) --batch-size 50 --limit 50

mood: mood-classify mood-apply

mood-classify:
	uv run ingest/classify-moods/classify.py --jsonl $(MOOD_JSONL) --resume

mood-apply:
	uv run ingest/classify-moods/apply.py --jsonl $(MOOD_JSONL)

# ============================================================
# Web
# ============================================================

web: web-install
	cd web && pnpm dev $(ARGS)

web-remote: web-install
	cd web && REMOTE=1 pnpm dev

web-install:
	cd web && pnpm install

web-build: web-install
	cd web && pnpm build

test:
	cd web && pnpm test:run

lint:
	cd web && pnpm lint

clean:
	rm -rf web/dist

# ============================================================
# Dev / Testing
# ============================================================

# Path to TGIF dataset TSV file (for local testing)
TGIF_TSV ?= $(HOME)/Documents/antfly/datasets/TGIF-Release/data/tgif-v1.0.tsv

# Quick prompt iteration test (processes N media items from local TGIF TSV)
test-prompt:
	@if [ ! -f "$(TGIF_TSV)" ]; then \
		echo "Error: TGIF dataset not found at $(TGIF_TSV)"; \
		exit 1; \
	fi
	@mkdir -p ingest/image-to-text/output
	uv run ingest/image-to-text/describe.py \
		--source tgif \
		--tsv "$(TGIF_TSV)" \
		--output "ingest/image-to-text/output/test_descriptions.jsonl" \
		--prompt "ingest/image-to-text/prompt.txt" \
		--workers 5 \
		--limit $(N)
	@echo ""
	@echo "=== Sample output (first item) ==="
	@head -1 ingest/image-to-text/output/test_descriptions.jsonl | python3 -m json.tool

proxy:
	python3 scripts/local_proxy.py
