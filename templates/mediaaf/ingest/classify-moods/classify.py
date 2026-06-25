#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["httpx", "pyyaml"]
# ///
"""
Classify GIF mood strings into 15 emoji categories using an LLM.

Extracts unique mood strings from the descriptions JSONL, batches them
to OpenRouter (GPT OSS 120B), and produces:
  - mood_mapping.json: { mood_string: category }
  - mood_by_id.jsonl: { "id": ..., "mood_emoji": ... } per doc

Usage:
    uv run ingest/classify-moods/classify.py --jsonl ingest/image-to-text/output/descriptions-gemini-2.5-flash-lite.jsonl
    uv run ingest/classify-moods/classify.py --jsonl ... --resume
    uv run ingest/classify-moods/classify.py --jsonl ... --dry-run   # show unique moods without calling API
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path

# Emoji taxonomy: 15 categories
CATEGORIES = {
    "happy":     {"emoji": "😊", "label": "Happy",     "keywords": ["joyful", "cheerful", "happy", "delighted", "positive", "lighthearted", "joy", "elated", "gleeful", "upbeat", "heartwarming"]},
    "wholesome": {"emoji": "🥰", "label": "Wholesome", "keywords": ["wholesome", "cute", "sweet", "adorable", "tender", "endearing", "precious", "loving", "affection"]},
    "playful":   {"emoji": "😜", "label": "Playful",   "keywords": ["playful", "silly", "goofy", "funny", "humorous", "comedic", "slapstick", "witty", "lighthearted-humor"]},
    "sassy":     {"emoji": "😏", "label": "Sassy",     "keywords": ["sarcastic", "smug", "dismissive", "judgmental", "unimpressed", "snarky", "condescending", "shady", "eye-roll", "sass"]},
    "chaotic":   {"emoji": "🤪", "label": "Chaotic",   "keywords": ["chaotic", "mischievous", "wild", "absurd", "unhinged", "crazy", "mayhem", "anarchic", "reckless"]},
    "shocked":   {"emoji": "😱", "label": "Shocked",   "keywords": ["surprised", "shocked", "stunned", "astonished", "disbelief", "jaw-drop", "startled", "bewildered"]},
    "angry":     {"emoji": "😤", "label": "Angry",     "keywords": ["frustrated", "annoyed", "angry", "exasperated", "furious", "irritated", "rage", "hostile", "aggravated", "disgusted", "repulsed", "revolted", "hateful"]},
    "awkward":   {"emoji": "😬", "label": "Awkward",    "keywords": ["awkward", "cringe", "uncomfortable", "embarrassing", "relatable", "self-conscious", "sheepish", "facepalm"]},
    "sad":       {"emoji": "😢", "label": "Sad",        "keywords": ["sad", "emotional", "distressed", "melancholy", "tearful", "bittersweet", "somber", "heartbroken", "grief"]},
    "scared":    {"emoji": "😨", "label": "Scared",     "keywords": ["anxious", "panicked", "tense", "scared", "unsettling", "menacing", "fearful", "nervous", "dread", "creepy", "horror"]},
    "cool":      {"emoji": "😎", "label": "Cool",       "keywords": ["confident", "cool", "determined", "defiant", "triumphant", "badass", "bold", "empowered", "powerful", "fierce"]},
    "confused":  {"emoji": "🤔", "label": "Confused",   "keywords": ["confused", "contemplative", "curious", "skeptical", "puzzled", "perplexed", "questioning", "thoughtful", "pondering"]},
    "flirty":    {"emoji": "😍", "label": "Flirty",     "keywords": ["flirty", "romantic", "affectionate", "seductive", "intimate", "sultry", "loving", "desire", "attraction"]},
    "excited":   {"emoji": "🥳", "label": "Excited",    "keywords": ["excited", "celebratory", "enthusiastic", "triumphant", "hype", "ecstatic", "thrilled", "pumped", "elation"]},
    "chill":     {"emoji": "😴", "label": "Chill",      "keywords": ["neutral", "peaceful", "relaxed", "calm", "bored", "nonchalant", "zen", "indifferent", "serene", "apathetic"]},
}

SYSTEM_PROMPT = """You are a mood classifier. Given a list of mood strings from GIF descriptions, classify each into exactly one of these 15 categories:

Categories:
- happy: joyful, cheerful, happy, delighted, positive, lighthearted
- wholesome: wholesome, cute, sweet, adorable, tender, heartwarming
- playful: playful, silly, goofy, funny, humorous, comedic, slapstick
- sassy: sarcastic, smug, dismissive, judgmental, unimpressed, snarky, shady, incredulous, eye-roll
- chaotic: chaotic, mischievous, wild, absurd, unhinged, crazy
- shocked: surprised, shocked, stunned, astonished, disbelief
- angry: frustrated, annoyed, angry, exasperated, furious, rage, disgusted, repulsed, hateful
- awkward: awkward, cringe, uncomfortable, embarrassing, relatable, facepalm, anxious, concerned, nervous, self-conscious
- sad: sad, emotional, distressed, melancholy, tearful, bittersweet, somber, despairing, vulnerable, resigned
- scared: scared, terrified, panicked, horror, fearful, dread, menacing, unsettling, creepy (reserve for actual fear/terror — mild anxiety or worry is awkward, not scared)
- cool: confident, cool, determined, defiant, triumphant, badass, empowered
- confused: confused, contemplative, curious, skeptical, puzzled, thoughtful, perplexed, bewildered, questioning
- flirty: flirty, romantic, affectionate, seductive, intimate
- excited: excited, celebratory, enthusiastic, triumphant, hype, ecstatic, thrilled, pumped, energetic, overjoyed
- chill: neutral, peaceful, relaxed, calm, bored, nonchalant, indifferent, serene, tired, apathetic, zen

Rules:
- Each mood string gets exactly one category
- For compound moods (e.g. "chaotic-funny"), pick the dominant emotional tone
- "Funny" and "dramatic" are modifiers, not categories — classify by the underlying emotion
- If a mood has multiple emotions (e.g. "confident, dismissive, celebratory"), pick the strongest one
- "anxious" or "concerned" without real fear/terror → awkward, not scared
- "skeptical", "incredulous", "unimpressed" → sassy or confused, never scared
- Output valid JSON only: an object mapping each mood string to its category value

Example input: ["joyful-lighthearted", "sarcastic-unimpressed", "chaotic-funny"]
Example output: {"joyful-lighthearted": "happy", "sarcastic-unimpressed": "sassy", "chaotic-funny": "chaotic"}"""

BATCH_SIZE = 100


def load_api_key() -> str:
    """Load OpenRouter API key."""
    if os.environ.get("OPENROUTER_API_KEY"):
        return os.environ["OPENROUTER_API_KEY"]
    key_path = Path.home() / ".tokens/openrouter_api_key"
    if key_path.exists():
        return key_path.read_text().strip().split()[0]
    raise ValueError("No OpenRouter API key. Set OPENROUTER_API_KEY or create ~/.tokens/openrouter_api_key")


def extract_unique_moods(jsonl_path: str) -> tuple[dict[str, list[str]], int]:
    """Extract unique mood strings and map them to document IDs.

    Returns (mood_to_ids, total_docs).
    """
    mood_to_ids: dict[str, list[str]] = {}
    total = 0
    with open(jsonl_path) as f:
        for line in f:
            total += 1
            doc = json.loads(line)
            mood = doc.get("mood", "").strip()
            doc_id = doc.get("id", "")
            if mood and doc_id:
                mood_to_ids.setdefault(mood, []).append(doc_id)
    return mood_to_ids, total


def keyword_fallback(mood: str) -> str:
    """Classify a mood string using keyword matching as fallback."""
    mood_lower = mood.lower().replace("-", " ").replace(",", " ")
    words = set(mood_lower.split())

    best_category = "playful"  # default
    best_score = 0

    for category, info in CATEGORIES.items():
        score = sum(1 for kw in info["keywords"] if kw in words or kw in mood_lower)
        if score > best_score:
            best_score = score
            best_category = category

    return best_category


def classify_batch(api_key: str, moods: list[str], model: str) -> tuple[dict[str, str], int, int]:
    """Classify a batch of mood strings via OpenRouter.

    Returns (mapping, input_tokens, output_tokens).
    """
    import httpx

    user_msg = json.dumps(moods)

    for attempt in range(3):
        try:
            resp = httpx.post(
                "https://openrouter.ai/api/v1/chat/completions",
                headers={"Authorization": f"Bearer {api_key}"},
                json={
                    "model": model,
                    "messages": [
                        {"role": "system", "content": SYSTEM_PROMPT},
                        {"role": "user", "content": user_msg},
                    ],
                    "max_tokens": 16384,
                    "temperature": 0.1,
                },
                timeout=120.0,
            )
            if resp.status_code in (429, 500, 502, 503):
                wait = 10 * (attempt + 1)
                print(f"  OpenRouter {resp.status_code}, retrying in {wait}s ({attempt + 1}/3)")
                time.sleep(wait)
                continue
            resp.raise_for_status()
            data = resp.json()

            if "choices" not in data:
                error = data.get("error", data)
                print(f"  API error: {error}", file=sys.stderr)
                return {}, 0, 0

            usage = data.get("usage", {})
            input_tokens = usage.get("prompt_tokens", 0)
            output_tokens = usage.get("completion_tokens", 0)

            content = data["choices"][0]["message"]["content"]
            if not content:
                if attempt < 2:
                    time.sleep(5)
                    continue
                return {}, 0, 0
            # Strip markdown fences if present
            content = content.strip()
            if content.startswith("```"):
                lines = content.split("\n")
                lines = lines[1:]  # remove ```json
                while lines and lines[-1].strip().startswith("`"):
                    lines = lines[:-1]
                content = "\n".join(lines)

            mapping = json.loads(content)

            # Validate: all values must be valid categories
            valid = set(CATEGORIES.keys())
            cleaned = {}
            for mood_str, cat in mapping.items():
                if cat in valid:
                    cleaned[mood_str] = cat
                else:
                    # Try keyword fallback for invalid category
                    cleaned[mood_str] = keyword_fallback(mood_str)

            return cleaned, input_tokens, output_tokens

        except json.JSONDecodeError as e:
            print(f"  JSON parse error: {e}", file=sys.stderr)
            if attempt < 2:
                time.sleep(5)
                continue
            return {}, 0, 0
        except Exception as e:
            print(f"  Error: {e}", file=sys.stderr)
            if attempt < 2:
                time.sleep(10 * (attempt + 1))
                continue
            return {}, 0, 0

    return {}, 0, 0


def main():
    parser = argparse.ArgumentParser(description="Classify GIF moods into emoji categories")
    parser.add_argument("--jsonl", required=True, help="Path to descriptions JSONL file")
    parser.add_argument("--output-dir", type=Path, default=Path(__file__).parent / "output",
                        help="Output directory")
    parser.add_argument("--model", default="openai/gpt-5-nano",
                        help="OpenRouter model ID")
    parser.add_argument("--resume", action="store_true", help="Resume from existing mood_mapping.json")
    parser.add_argument("--dry-run", action="store_true", help="Just show unique moods, don't call API")
    parser.add_argument("--batch-size", type=int, default=BATCH_SIZE, help=f"Moods per API call (default: {BATCH_SIZE})")
    parser.add_argument("--workers", type=int, default=10, help="Number of concurrent API requests (default: 10)")
    parser.add_argument("--limit", type=int, default=0, help="Limit unique moods to classify (0=all)")
    args = parser.parse_args()

    args.output_dir.mkdir(parents=True, exist_ok=True)

    mapping_path = args.output_dir / "mood_mapping.json"
    by_id_path = args.output_dir / "mood_by_id.jsonl"

    # Extract unique moods
    print(f"Reading {args.jsonl}...")
    mood_to_ids, total_docs = extract_unique_moods(args.jsonl)
    print(f"  {total_docs:,} docs, {len(mood_to_ids):,} unique mood strings")

    if args.dry_run:
        # Sort by frequency
        by_freq = sorted(mood_to_ids.items(), key=lambda x: -len(x[1]))
        print(f"\nTop 50 moods by frequency:")
        for mood, ids in by_freq[:50]:
            print(f"  {len(ids):5d}  {mood}")
        print(f"\nBottom 10:")
        for mood, ids in by_freq[-10:]:
            print(f"  {len(ids):5d}  {mood}")
        return

    # Load existing mapping if resuming
    existing_mapping: dict[str, str] = {}
    if args.resume and mapping_path.exists():
        with open(mapping_path) as f:
            existing_mapping = json.load(f)
        print(f"  Loaded {len(existing_mapping):,} existing classifications")

    # Find moods that still need classification
    to_classify = [m for m in mood_to_ids if m not in existing_mapping]
    if args.limit and len(to_classify) > args.limit:
        to_classify = to_classify[:args.limit]
    print(f"  {len(to_classify):,} moods to classify")

    if not to_classify:
        print("All moods already classified!")
    else:
        import threading
        from concurrent.futures import ThreadPoolExecutor, as_completed

        api_key = load_api_key()
        total_input_tokens = 0
        total_output_tokens = 0
        start = time.time()
        lock = threading.Lock()
        completed_batches = 0

        # Batch classify with threading
        batches = [to_classify[i:i + args.batch_size] for i in range(0, len(to_classify), args.batch_size)]
        print(f"  {len(batches)} batches of up to {args.batch_size} with {args.workers} workers")

        with ThreadPoolExecutor(max_workers=args.workers) as executor:
            futures = {
                executor.submit(classify_batch, api_key, batch, args.model): batch
                for batch in batches
            }

            for future in as_completed(futures):
                batch = futures[future]
                result, in_tok, out_tok = future.result()

                with lock:
                    total_input_tokens += in_tok
                    total_output_tokens += out_tok
                    completed_batches += 1

                    for mood_str in batch:
                        if mood_str in result:
                            existing_mapping[mood_str] = result[mood_str]
                        else:
                            existing_mapping[mood_str] = keyword_fallback(mood_str)

                    elapsed = time.time() - start
                    print(f"\r  {completed_batches}/{len(batches)} batches — "
                          f"{len(existing_mapping):,} classified ({elapsed:.1f}s)\033[K",
                          end="", flush=True)

                    # Save periodically for resumability
                    if completed_batches % 10 == 0 or completed_batches == len(batches):
                        with open(mapping_path, "w") as f:
                            json.dump(existing_mapping, f, indent=2, ensure_ascii=False)

        print()
        elapsed = time.time() - start
        print(f"Classification complete in {elapsed:.1f}s")
        print(f"Tokens: {total_input_tokens:,} in / {total_output_tokens:,} out")

        # Load pricing from models.yaml if available
        import yaml
        pricing_path = Path(__file__).parent.parent / "image-to-text/models.yaml"
        if pricing_path.exists():
            with open(pricing_path) as f:
                models_config = yaml.safe_load(f)
            model_pricing = {m["id"]: (m.get("input_per_m", 0), m.get("output_per_m", 0))
                             for m in models_config.get("models", [])}
            ip, op = model_pricing.get(args.model, (0, 0))
            if ip > 0 or op > 0:
                cost = (total_input_tokens * ip + total_output_tokens * op) / 1_000_000
                print(f"Cost: ${cost:.4f}")

    # Save final mapping
    with open(mapping_path, "w") as f:
        json.dump(existing_mapping, f, indent=2, ensure_ascii=False)
    print(f"Saved mapping: {mapping_path} ({len(existing_mapping):,} entries)")

    # Generate mood_by_id.jsonl
    print(f"Generating {by_id_path}...")
    count = 0
    missing = 0
    with open(by_id_path, "w") as out:
        for mood_str, doc_ids in mood_to_ids.items():
            category = existing_mapping.get(mood_str)
            if not category:
                category = keyword_fallback(mood_str)
                missing += 1
            for doc_id in doc_ids:
                out.write(json.dumps({"id": doc_id, "mood_emoji": category}) + "\n")
                count += 1
    print(f"  {count:,} entries written ({missing} used keyword fallback)")

    # Distribution summary
    dist: dict[str, int] = {}
    for mood_str, doc_ids in mood_to_ids.items():
        cat = existing_mapping.get(mood_str, keyword_fallback(mood_str))
        dist[cat] = dist.get(cat, 0) + len(doc_ids)
    print(f"\nDistribution across {total_docs:,} docs:")
    for cat in sorted(dist, key=lambda c: -dist[c]):
        info = CATEGORIES[cat]
        pct = dist[cat] / total_docs * 100
        print(f"  {info['emoji']} {info['label']:10s} {dist[cat]:6,d} ({pct:.1f}%)")


if __name__ == "__main__":
    main()
