import { useState, useCallback, useEffect, useRef } from 'react';

interface SearchBoxProps {
  onSearch: (query: string) => void;
  isLoading?: boolean;
  initialValue?: string;
  activeTag?: string | null;
  onRemoveTag?: () => void;
}

export function SearchBox({ onSearch, isLoading, initialValue = '', activeTag, onRemoveTag }: SearchBoxProps) {
  const [query, setQuery] = useState(initialValue);
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // Debounce the query
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query);
    }, 300);
    return () => clearTimeout(timer);
  }, [query]);

  // Trigger search when debounced query changes
  useEffect(() => {
    if (debouncedQuery.trim() || activeTag) {
      onSearch(debouncedQuery);
    }
  }, [debouncedQuery, activeTag, onSearch]);

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (query.trim() || activeTag) {
        onSearch(query);
      }
    },
    [query, activeTag, onSearch]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      // Backspace at start of empty input removes the tag bubble
      if (e.key === 'Backspace' && activeTag && query === '') {
        e.preventDefault();
        onRemoveTag?.();
      }
    },
    [activeTag, query, onRemoveTag]
  );

  return (
    <form onSubmit={handleSubmit} className="w-full max-w-2xl mx-auto">
      <div
        className="relative flex items-center gap-1.5 w-full px-3 py-2 rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--background))] focus-within:ring-2 focus-within:ring-[hsl(var(--ring))] focus-within:border-transparent"
        onClick={() => inputRef.current?.focus()}
      >
        {activeTag && (
          <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-[hsl(var(--ring))]/15 text-[hsl(var(--ring))] text-sm font-medium shrink-0">
            tag:{activeTag}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onRemoveTag?.();
              }}
              className="ml-0.5 rounded-full p-0.5 hover:bg-[hsl(var(--ring))]/25 transition-colors"
              title="Remove tag"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </span>
        )}
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={activeTag ? "Add text search..." : "Search for media... (e.g., 'cat playing', 'happy dance', 'thumbs up')"}
          className="flex-1 min-w-0 py-1 text-lg bg-transparent text-[hsl(var(--foreground))] placeholder:text-[hsl(var(--muted-foreground))] focus:outline-none"
          autoFocus
        />
        {isLoading && (
          <div className="shrink-0">
            <div className="w-5 h-5 border-2 border-[hsl(var(--muted-foreground))] border-t-transparent rounded-full animate-spin" />
          </div>
        )}
      </div>
    </form>
  );
}
