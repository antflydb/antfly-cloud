import { useEffect, useCallback } from "react";

interface AboutModalProps {
  onClose: () => void;
}

export function AboutModal({ onClose }: AboutModalProps) {
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

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto"
      onClick={onClose}
    >
      <div className="fixed inset-0 bg-black/60" />

      <div
        className="relative z-10 w-full max-w-2xl my-8 mx-4 bg-[hsl(var(--card))] rounded-xl border border-[hsl(var(--border))] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-3 right-3 z-10 p-1.5 rounded-lg hover:bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))] transition-colors"
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

        <div className="p-6 space-y-5">
          <h2 className="text-xl font-bold text-[hsl(var(--foreground))]">
            About MediaAF
          </h2>

          <p className="text-sm text-[hsl(var(--foreground))]">
            MediaAF is a semantic media search engine powered by{" "}
            <a
              href="https://antfly.io"
              className="underline hover:opacity-80"
              target="_blank"
              rel="noopener noreferrer"
            >
              Antfly
            </a>
            . It indexes over 106,000 media items and lets you search them by meaning, not just keywords.
          </p>

          <div className="space-y-3">
            <h3 className="text-sm font-semibold text-[hsl(var(--foreground))] uppercase tracking-wide">
              How It Works
            </h3>

            <div className="space-y-2 text-sm text-[hsl(var(--foreground))]">
              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">1.</span>
                <p>
                  <strong>Data:</strong> media items come from the{" "}
                  <a
                    href="https://github.com/raingo/Tmedia-Release"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Tmedia dataset
                  </a>{" "}
                  (~100k animated media items originally from Tumblr, created by{" "}
                  <a
                    href="https://arxiv.org/abs/1604.02748"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Li et al
                  </a>
                  ) and the{" "}
                  <a
                    href="http://lucas.maystre.ch/gifgif-data"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    MIT Media Lab GifGif
                  </a>{" "}
                  dataset (~6k media items with crowd-sourced emotion ratings).
                </p>
              </div>

              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">2.</span>
                <p>
                  <strong>AI Descriptions:</strong> Multiple frames are extracted from each media
                  and sent to{" "}
                  <strong>Gemini 2.5 Flash Lite</strong> to generate rich structured descriptions
                  including mood, tags, source identification, use cases, and content ratings.
                </p>
              </div>

              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">3.</span>
                <p>
                  <strong>Search:</strong> Descriptions are indexed in{" "}
                  <a
                    href="https://antfly.io/docs"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Antfly
                  </a>{" "}
                  with text embeddings (
                  <a
                    href="https://huggingface.co/BAAI/bge-small-en-v1.5"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    BAAI/bge-small-en-v1.5
                  </a>
                  ) via{" "}
                  <a
                    href="https://antfly.io/termite"
                    className="underline hover:opacity-80"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Termite
                  </a>
                  . Queries run both semantic vector search and full-text search, merged
                  with reciprocal rank fusion (RRF).
                </p>
              </div>
            </div>
          </div>

          <div className="space-y-3">
            <h3 className="text-sm font-semibold text-[hsl(var(--foreground))] uppercase tracking-wide">
              Search Tips
            </h3>
            <div className="space-y-2 text-sm text-[hsl(var(--foreground))]">
              <p>
                <code className="px-1.5 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">cat playing</code>{" "}
                — Semantic + full-text search across all descriptions
              </p>
              <p>
                <code className="px-1.5 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">"spongebob squarepants"</code>{" "}
                — Exact phrase match combined with semantic search
              </p>
              <p>
                <code className="px-1.5 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">tag:funny</code>{" "}
                — Filter by exact tag
              </p>
              <p>
                <code className="px-1.5 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">dancing tag:celebration</code>{" "}
                — Combine text search with tag filter
              </p>
              <p>
                <code className="px-1.5 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">corgi -tag:anime</code>{" "}
                — Exclude results with a specific tag
              </p>
              <p>
                Use the mood emoji bar to filter by emotional tone
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <h3 className="text-sm font-semibold text-[hsl(var(--foreground))] uppercase tracking-wide">
              Stack
            </h3>
            <div className="flex flex-wrap gap-2">
              {[
                "Antfly",
                "Termite",
                "Gemini 2.5 Flash Lite",
                "BAAI/bge-small-en-v1.5",
                "React",
                "TypeScript",
                "Tailwind CSS",
                "Vite",
              ].map((tech) => (
                <span
                  key={tech}
                  className="px-2 py-0.5 rounded-full bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))] text-xs"
                >
                  {tech}
                </span>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
