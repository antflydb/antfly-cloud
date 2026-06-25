# MediaAF Makefile
#
# Prerequisites:
#   - Antfly running: antfly swarm
#   - Text embedding model: antflycli termite pull BAAI/bge-small-en-v1.5 --type embedder

# Load .env file if it exists
-include .env
export

# Antfly API URL
ANTFLY_URL ?= http://localhost:8080/api/v1

# Ingest settings
INGEST_BATCH_SIZE ?= 50
INGEST_TABLE ?= mediaaf

# Describe settings
WORKERS ?= 20
MODEL ?= gemini-3.1-flash-lite-preview
DESCRIBE_OUTPUT_DIR := ingest/image-to-text/output
N ?= 100
MEDIA_BASE_URL ?=

# Sources
SOURCES_DIR := sources

.PHONY: help setup proxy describe-sample scrape upload upload-all describe compare review ingest status web web-remote web-install web-build test lint clean test-prompt mood-test mood

help:
	@echo "MediaAF - Available targets:"
	@echo ""
	@echo "  --- Pipeline ---"
	@echo "  scrape SRC=X           - Discover media items for source X (build manifest)"
	@echo "  upload SRC=X           - Upload source X media to R2"
	@echo "  upload-all             - Upload all sources to R2"
	@echo "  describe N=100         - Generate descriptions from R2 (single model)"
	@echo "  describe N=100 SRC=X   - Describe only source X (R2 prefix filter)"
	@echo "  describe MODEL=X       - Use a specific model (default: $(MODEL))"
	@echo "  describe-sample        - Print transform smoke-test guidance"
	@echo "  compare N=20 MODELS=.. - Compare models side-by-side"
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
	@echo "MediaAF sample description depends on model API credentials."
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
# Models prefixed with "openrouter:" use the OpenRouter backend
describe:
	@test -n "$(R2_BUCKET)" || (echo "Error: R2_BUCKET not set" && exit 1)
	@mkdir -p $(DESCRIBE_OUTPUT_DIR)
	$(eval DESCRIBE_BACKEND := $(if $(filter openrouter:%,$(MODEL)),openrouter,genai))
	$(eval DESCRIBE_MODEL := $(patsubst openrouter:%,%,$(MODEL)))
	$(eval DESCRIBE_SAFE_NAME := $(subst /,-,$(DESCRIBE_MODEL)))
	uv run ingest/image-to-text/describe.py \
		--source r2 \
		--r2-bucket "$(R2_BUCKET)" \
		$(if $(SRC),--r2-prefix "sources/$(SRC)/") \
		--backend $(DESCRIBE_BACKEND) \
		--model "$(DESCRIBE_MODEL)" \
		--output "$(DESCRIBE_OUTPUT_DIR)/descriptions-$(DESCRIBE_SAFE_NAME).jsonl" \
		--prompt "ingest/image-to-text/prompt.txt" \
		--workers $(WORKERS) \
		--limit $(N) \
		--resume \
		$(if $(ONLY_UNPROCESSED_BY),--only-unprocessed-by "$(ONLY_UNPROCESSED_BY)")
	ln -sf descriptions-$(DESCRIBE_SAFE_NAME).jsonl $(DESCRIBE_OUTPUT_DIR)/descriptions-latest.jsonl
	@echo "Active model: $(DESCRIBE_MODEL)"
	$(MAKE) review

# Generate descriptions with multiple models for comparison
# Usage: make compare N=20 MODELS="gemini-3.1-flash-lite-preview,gemini-2.5-flash"
# Models prefixed with "openrouter:" use the OpenRouter backend (e.g. openrouter:google/gemma-3-4b-it)
compare:
	@test -n "$(R2_BUCKET)" || (echo "Error: R2_BUCKET not set" && exit 1)
	@test -n "$(MODELS)" || (echo "Usage: make compare N=20 MODELS=\"model1,model2\"" && exit 1)
	@mkdir -p $(DESCRIBE_OUTPUT_DIR)
	@compare_files=""; \
	for spec in $$(echo "$(MODELS)" | tr ',' ' '); do \
		echo ""; \
		case "$$spec" in \
			openrouter:*) \
				model=$${spec#openrouter:}; \
				backend=openrouter; \
				;; \
			*) \
				model=$$spec; \
				backend=genai; \
				;; \
		esac; \
		safe_name=$$(echo "$$model" | tr '/' '-'); \
		echo "=== Running model: $$model ($$backend) ==="; \
		uv run ingest/image-to-text/describe.py \
			--source r2 \
			--r2-bucket "$(R2_BUCKET)" \
			--backend "$$backend" \
			--model "$$model" \
			--output "$(DESCRIBE_OUTPUT_DIR)/descriptions-$$safe_name.jsonl" \
			--prompt "ingest/image-to-text/prompt.txt" \
			--workers $(WORKERS) \
			--limit $(N); \
		compare_files="$$compare_files $(DESCRIBE_OUTPUT_DIR)/descriptions-$$safe_name.jsonl"; \
	done; \
	uv run ingest/image-to-text/review.py \
		--input $$compare_files \
		--output "$(DESCRIBE_OUTPUT_DIR)/review.md"

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

# Quick prompt iteration test (processes N media items from local Tmedia)
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
