import { useState, useEffect, useCallback } from "react";
import type { GifResult } from "../lib/antfly";
import { MOOD_EMOJIS, type MoodValue } from "./MoodFilterBar";

interface GifDetailProps {
  gif: GifResult;
  onClose: () => void;
  hasActiveSearch?: boolean;
  onTagClick?: (tag: string) => void;
  onMoodClick?: (mood: MoodValue) => void;
}

// Fields to never display
const HIDDEN_FIELDS = new Set([
  "id",
  "gif_url",
  "score",
  "rank",
  "combined_text",
  "_embeddings",
  "attribution",
  "original_url",
  "mood_emoji",
]);

// Ordered list of AI fields (tags first)
const AI_FIELD_ORDER = ["tags", "literal", "source", "mood", "context", "rating", "names", "visual_style"];

// Ordered list of non-AI fields
const NON_AI_FIELD_ORDER = ["original_description", "description", "tumblr_id"];

// Display names
const FIELD_LABELS: Record<string, string> = {
  tags: "Tags",
  literal: "Description",
  source: "Source",
  mood: "Mood",
  context: "Use Case",
  rating: "Rating",
  names: "Names",
  visual_style: "Visual Style",
  original_description: "Original Caption",
  description: "Caption",
  tumblr_id: "Tumblr ID",
  _timestamp: "Retrieved At",
  timestamp: "Retrieved At",
  created_at: "Retrieved At",
};

// The model used for AI descriptions
const AI_MODEL_LABEL = "Gemini 2.5 Flash Lite";

function formatValue(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "object" && value !== null) return JSON.stringify(value);
  return String(value);
}

function fieldLabel(key: string): string {
  return FIELD_LABELS[key] ?? key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function renderField(
  key: string,
  value: unknown,
  onTagClick?: (tag: string) => void,
  moodEmoji?: string,
  onMoodClick?: (mood: MoodValue) => void,
) {
  const moodInfo = key === "mood" && moodEmoji
    ? MOOD_EMOJIS.find(m => m.value === moodEmoji)
    : null;

  return (
    <div key={key}>
      <dt className="text-xs font-medium uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
        {fieldLabel(key)}
      </dt>
      <dd className="mt-0.5 text-sm text-[hsl(var(--foreground))]">
        {key === "tags" && Array.isArray(value) ? (
          <div className="flex flex-wrap gap-1.5 mt-1">
            {(value as string[]).map((tag) => (
              <button
                key={tag}
                onClick={() => onTagClick?.(tag)}
                className="px-2 py-0.5 rounded-full bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))] text-xs hover:bg-[hsl(var(--ring))] hover:text-white transition-colors cursor-pointer"
                title={`Search for tag:${tag}`}
              >
                {tag}
              </button>
            ))}
          </div>
        ) : (
          <span className="inline-flex items-center gap-1.5">
            {formatValue(value)}
            {moodInfo && (
              <button
                onClick={() => onMoodClick?.(moodInfo.value)}
                className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full bg-[hsl(var(--muted))] text-xs hover:bg-[hsl(var(--ring))] hover:text-white transition-colors cursor-pointer"
                title={`Filter by ${moodInfo.label}`}
              >
                <span>{moodInfo.emoji}</span>
                <span>{moodInfo.label}</span>
              </button>
            )}
          </span>
        )}
      </dd>
    </div>
  );
}

export function GifDetail({ gif, onClose, hasActiveSearch, onTagClick, onMoodClick }: GifDetailProps) {
  const [copied, setCopied] = useState<boolean | "error">(false);

  const handleCopyUrl = useCallback(async () => {
    try {
      // Try rich copy (HTML + text) first — works on desktop Chrome/Firefox
      if (typeof ClipboardItem !== "undefined" && navigator.clipboard?.write) {
        const html = `<img src="${gif.gif_url}">`;
        const htmlBlob = new Blob([html], { type: "text/html" });
        const textBlob = new Blob([gif.gif_url], { type: "text/plain" });
        await navigator.clipboard.write([
          new ClipboardItem({
            "text/html": htmlBlob,
            "text/plain": textBlob,
          }),
        ]);
      } else {
        // Fallback: plain text copy (broader mobile/WebKit support)
        await navigator.clipboard.writeText(gif.gif_url);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Final fallback: execCommand for older browsers
      try {
        const textarea = document.createElement("textarea");
        textarea.value = gif.gif_url;
        textarea.style.position = "fixed";
        textarea.style.opacity = "0";
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand("copy");
        document.body.removeChild(textarea);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch (err) {
        console.error("Failed to copy:", err);
        setCopied("error");
        setTimeout(() => setCopied(false), 2000);
      }
    }
  }, [gif.gif_url]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener("keydown", handleKeyDown);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = "";
    };
  }, [handleKeyDown]);

  const gifData = gif;

  // Collect ordered AI fields that have values
  const aiFields = AI_FIELD_ORDER.filter(
    (key) => gifData[key] != null && gifData[key] !== "",
  );

  // Collect ordered non-AI fields that have values
  // Skip "description" if "original_description" is present (same data, different field names across pipelines)
  const nonAiFields = NON_AI_FIELD_ORDER.filter((key) => {
    if (key === "description" && gifData["original_description"] != null && gifData["original_description"] !== "") {
      return false;
    }
    return gifData[key] != null && gifData[key] !== "";
  });

  // Collect any remaining fields not in explicit lists (excluding hidden, AI, non-AI, and timestamp-like)
  const timestampKeys = new Set(["_timestamp", "timestamp", "created_at"]);
  const explicitKeys = new Set([...AI_FIELD_ORDER, ...NON_AI_FIELD_ORDER, ...HIDDEN_FIELDS, ...timestampKeys]);
  const extraFields = Object.keys(gifData).filter(
    (key) => !explicitKeys.has(key) && gifData[key] != null && gifData[key] !== "",
  );

  // Find timestamp field (try multiple names)
  const timestampKey = ["_timestamp", "timestamp", "created_at"].find(
    (k) => gifData[k] != null && gifData[k] !== "",
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto"
      onClick={onClose}
    >
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/60" />

      {/* Content */}
      <div
        className="relative z-10 w-full max-w-2xl my-8 mx-4 bg-[hsl(var(--card))] rounded-xl border border-[hsl(var(--border))] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-3 right-3 z-10 p-1.5 rounded-lg bg-black/50 hover:bg-black/70 text-white transition-colors"
          title="Close (Esc)"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>

        {/* media image */}
        <div className="relative">
          <img
            src={gif.gif_url}
            alt={gif.description}
            className="w-full h-auto rounded-t-xl"
          />
          {/* Score badge - only show during active search */}
          {hasActiveSearch && (
            <div className="absolute bottom-3 left-3 px-2 py-1 bg-black/70 rounded text-white text-xs font-mono">
              Score: {gif.score.toFixed(3)}
              {gif.rank != null && ` (#${gif.rank})`}
            </div>
          )}
        </div>

        {/* Copy URL bar — above the fold */}
        <div className="px-5 pt-4">
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-[hsl(var(--muted))] text-xs font-mono text-[hsl(var(--muted-foreground))] overflow-hidden">
            <span className="truncate flex-1">{gif.gif_url}</span>
            <button
              onClick={handleCopyUrl}
              className="shrink-0 px-2 py-1 rounded bg-[hsl(var(--background))] hover:bg-[hsl(var(--ring))] hover:text-white text-[hsl(var(--foreground))] text-xs transition-colors"
            >
              {copied === 'error' ? 'Failed!' : copied ? 'Copied!' : 'Copy URL'}
            </button>
          </div>
        </div>

        <div className="p-5 space-y-4">
          {/* AI-generated fields section */}
          {aiFields.length > 0 && (
            <div className="border-l-4 border-green-700 pl-4 space-y-3">
              <p className="text-xs font-medium text-[hsl(var(--muted-foreground))]">
                Description by {AI_MODEL_LABEL}
              </p>
              {aiFields.map((key) => renderField(key, gifData[key], onTagClick, key === "mood" ? (gifData.mood_emoji as string) : undefined, onMoodClick))}
            </div>
          )}

          {/* Non-AI fields */}
          {nonAiFields.length > 0 && (
            <div className="space-y-3">
              {nonAiFields.map((key) => renderField(key, gifData[key]))}
            </div>
          )}

          {/* Any extra fields not in our explicit lists */}
          {extraFields.length > 0 && (
            <div className="space-y-3">
              {extraFields.map((key) => renderField(key, gifData[key]))}
            </div>
          )}

          {/* Timestamp at the bottom */}
          {timestampKey && (
            <div className="space-y-3">
              {renderField(timestampKey, gifData[timestampKey])}
            </div>
          )}
        </div>

        {/* Attribution + ID */}
        <div className="px-5 pb-4 space-y-2">
          {gif.attribution && (
            <div className="text-xs text-[hsl(var(--muted-foreground))]">
              Dataset:{" "}
              {gif.attribution.startsWith("http") ? (
                <a
                  href={gif.attribution}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-[hsl(var(--ring))] hover:underline"
                >
                  {gif.attribution}
                </a>
              ) : (
                <span>{gif.attribution}</span>
              )}
            </div>
          )}
          {typeof gifData.original_url === "string" && gifData.original_url && (
            <div className="text-xs text-[hsl(var(--muted-foreground))]">
              Original URL:{" "}
              <a
                href={gifData.original_url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-[hsl(var(--ring))] hover:underline"
              >
                {new URL(gifData.original_url).hostname}
              </a>
            </div>
          )}
          {gif.id && (
            <div className="text-xs font-mono text-[hsl(var(--muted-foreground))]">
              ID: {gif.id}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
