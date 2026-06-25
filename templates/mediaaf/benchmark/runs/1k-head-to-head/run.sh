#!/usr/bin/env bash
set -euo pipefail

# Resolve paths relative to this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Load .env from project root
if [ -f "$PROJECT_ROOT/.env" ]; then
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi
MANIFEST="$SCRIPT_DIR/manifest.json"
RESULTS_DIR="$SCRIPT_DIR/results"
DESCRIBE="$PROJECT_ROOT/ingest/image-to-text/describe.py"

mkdir -p "$RESULTS_DIR"

# Validate manifest exists
if [ ! -f "$MANIFEST" ]; then
    echo "Error: manifest.json not found. Run generate_manifest.py first."
    exit 1
fi

COUNT=$(python3 -c "import json; print(len(json.load(open('$MANIFEST'))))")
echo "Manifest: $COUNT GIFs"

# ---- Configuration ----
FRAMES=5
R2_BUCKET="${R2_BUCKET:-honeycomb-media}"

# Parse arguments
RUN_GEMINI=1
RUN_TERMITE=1
for arg in "$@"; do
    case $arg in
        --gemini-only) RUN_TERMITE=0 ;;
        --termite-only) RUN_GEMINI=0 ;;
    esac
done

# ---- Gemini API Run ----
if [ "$RUN_GEMINI" = 1 ]; then
    echo ""
    echo "===== Gemini Flash Lite (API) ====="
    time uv run "$DESCRIBE" \
        --source manifest \
        --manifest-file "$MANIFEST" \
        --r2-bucket "$R2_BUCKET" \
        --backend genai \
        --model gemini-2.0-flash-lite \
        --frames "$FRAMES" \
        --workers 20 \
        --limit 0 \
        --output "$RESULTS_DIR/gemini-1k-5frame.jsonl"
fi

# ---- Termite Run ----
if [ "$RUN_TERMITE" = 1 ]; then
    TERMITE_URL="${TERMITE_URL:-http://localhost:11433}"

    # Health check
    if ! curl -sf "$TERMITE_URL/openai/v1/models" > /dev/null 2>&1; then
        echo "Error: Termite not reachable at $TERMITE_URL"
        echo "Start it or set TERMITE_URL= to the correct address"
        exit 1
    fi

    echo ""
    echo "===== Termite (Local GPU) ====="
    time uv run "$DESCRIBE" \
        --source manifest \
        --manifest-file "$MANIFEST" \
        --r2-bucket "$R2_BUCKET" \
        --backend termite \
        --termite-url "$TERMITE_URL" \
        --frames "$FRAMES" \
        --workers 1 \
        --limit 0 \
        --output "$RESULTS_DIR/termite-1k-5frame.jsonl"
fi

echo ""
echo "Done. Results in: $RESULTS_DIR/"
echo "Run compare.py to generate analysis."
