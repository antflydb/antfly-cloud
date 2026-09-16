import type { GifResult } from "../lib/antfly";
import { GifCard } from "./GifCard";

interface GifGridProps {
  gifs: GifResult[];
  isLoading?: boolean;
  onGifClick?: (gif: GifResult) => void;
  hasActiveSearch?: boolean;
}

export function GifGrid({ gifs, isLoading, onGifClick, hasActiveSearch }: GifGridProps) {
  if (isLoading && gifs.length === 0) {
    // Loading skeleton - use masonry-style columns
    return (
      <div className="columns-2 sm:columns-3 md:columns-4 lg:columns-5 gap-4">
        {Array.from({ length: 20 }).map((_, i) => (
          <div
            key={i}
            className="mb-4 break-inside-avoid aspect-square bg-[hsl(var(--muted))] rounded-lg animate-pulse"
          />
        ))}
      </div>
    );
  }

  if (gifs.length === 0) {
    return (
      <div className="text-center py-12 text-[hsl(var(--muted-foreground))]">
        <p className="text-lg">No media items found</p>
        <p className="text-sm mt-2">Try a different search term</p>
      </div>
    );
  }

  return (
    <div className="columns-2 sm:columns-3 md:columns-4 lg:columns-5 gap-4">
      {gifs.map((gif, index) => (
        <GifCard key={gif.id || `gif-${index}`} gif={gif} onClick={() => onGifClick?.(gif)} hasActiveSearch={hasActiveSearch} />
      ))}
    </div>
  );
}
