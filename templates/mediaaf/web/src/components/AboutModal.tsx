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
            MediaAF is a locally hostable semantic search starter for image and
            moving-image corpora. It uses Antfly Cloud as the search backend while
            keeping the Cloud API key behind a local proxy instead of exposing it
            in the browser.
          </p>

          <div className="space-y-3">
            <h3 className="text-sm font-semibold text-[hsl(var(--foreground))] uppercase tracking-wide">
              How It Works
            </h3>

            <div className="space-y-2 text-sm text-[hsl(var(--foreground))]">
              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">1.</span>
                <p>
                  <strong>Transform:</strong> each media item is converted into a compact
                  package for a description model, such as representative frames for an
                  image or short clip.
                </p>
              </div>

              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">2.</span>
                <p>
                  <strong>Describe:</strong> the model writes structured text about meaning,
                  mood, tags, visible actions, possible use cases, and safety/rating signals.
                </p>
              </div>

              <div className="flex gap-3">
                <span className="text-[hsl(var(--muted-foreground))] font-mono shrink-0">3.</span>
                <p>
                  <strong>Search:</strong> those descriptions are stored in Antfly Cloud with
                  text embeddings over <code className="px-1 py-0.5 rounded bg-[hsl(var(--muted))] text-xs font-mono">combined_text</code>,
                  so users can search by meaning as well as exact tags or phrases.
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
                "Antfly Cloud",
                "Text embeddings",
                "Configurable LLM descriptions",
                "Local API-key proxy",
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
