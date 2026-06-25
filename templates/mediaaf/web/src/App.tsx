import { useState, useCallback, useEffect } from 'react';
import { SearchBox } from './components/SearchBox';
import { GifGrid } from './components/GifGrid';
import { GifDetail } from './components/GifDetail';
import { AboutModal } from './components/AboutModal';
import { MoodFilterBar, MOOD_EMOJIS, type MoodValue } from './components/MoodFilterBar';
import { searchGifs, getRandomGifs, getGifById, type GifResult } from './lib/antfly';

const TABLE_NAME = 'honeycomb';

function App() {
  const [gifs, setGifs] = useState<GifResult[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastQuery, setLastQuery] = useState('');
  const [initialLoadDone, setInitialLoadDone] = useState(false);
  const [selectedGif, setSelectedGif] = useState<GifResult | null>(null);
  const [searchKey, setSearchKey] = useState(0);
  const [showAbout, setShowAbout] = useState(false);
  const [totalGifs, setTotalGifs] = useState<number | null>(null);
  const [searchInput, setSearchInput] = useState('');
  const [activeTag, setActiveTag] = useState<string | null>(null);
  const [selectedMood, setSelectedMood] = useState<MoodValue | null>(null);
  const [isDark, setIsDark] = useState(() => {
    if (typeof window !== 'undefined') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    return false;
  });
  const [deepLinkPending, setDeepLinkPending] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    return !!params.get('gif');
  });

  // Toggle dark mode
  useEffect(() => {
    document.documentElement.classList.toggle('dark', isDark);
  }, [isDark]);

  // Sync selectedGif ↔ URL ?gif= param (skip while deep link is being resolved)
  useEffect(() => {
    if (deepLinkPending) return;
    const params = new URLSearchParams(window.location.search);
    if (selectedGif) {
      params.set('gif', selectedGif.id);
    } else {
      params.delete('gif');
    }
    const qs = params.toString();
    const newUrl = qs ? `${window.location.pathname}?${qs}` : window.location.pathname;
    window.history.replaceState(null, '', newUrl);
  }, [selectedGif, deepLinkPending]);

  // Load media items on mount, then open deep-linked media if any
  useEffect(() => {
    const abortController = new AbortController();
    const deepLinkGifId = new URLSearchParams(window.location.search).get('gif');

    const loadGifs = async () => {
      setIsLoading(true);
      setError(null);
      setGifs([]);
      setLastQuery('');
      try {
        const response = await getRandomGifs(TABLE_NAME);
        if (abortController.signal.aborted) return;
        setGifs(response.results);
        setTotalGifs(response.total);

        // Check for deep-linked media
        if (deepLinkGifId) {
          // Try to find it in loaded results first
          const found = response.results.find(g => g.id === deepLinkGifId);
          if (found) {
            setSelectedGif(found);
          } else {
            // Fetch it directly from Antfly
            const gif = await getGifById(TABLE_NAME, deepLinkGifId);
            if (abortController.signal.aborted) return;
            if (gif) {
              setSelectedGif(gif);
            } else if (import.meta.env.DEV) {
              console.warn(`Deep link: getGifById("${deepLinkGifId}") returned null`);
            }
          }
        }
        // Deep link resolved (or no deep link) — unlock URL sync
        setDeepLinkPending(false);
      } catch (err) {
        if (abortController.signal.aborted) return;
        console.error('Failed to load media items:', err);
        setError(err instanceof Error ? err.message : 'Failed to connect to Antfly');
        // Keep deepLinkPending=true so ?gif= param is preserved for retry
      } finally {
        if (!abortController.signal.aborted) {
          setIsLoading(false);
          setInitialLoadDone(true);
        }
      }
    };
    loadGifs();

    return () => abortController.abort();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Build the full query string from text input + active tag
  const buildQuery = useCallback((text: string, tag: string | null) => {
    const parts: string[] = [];
    if (tag) {
      const tagExpr = tag.includes(' ') ? `tag:"${tag}"` : `tag:${tag}`;
      parts.push(tagExpr);
    }
    if (text.trim()) {
      parts.push(text.trim());
    }
    return parts.join(' ');
  }, []);

  const handleSearch = useCallback(async (query: string, moodOverride?: MoodValue | null) => {
    const mood = moodOverride !== undefined ? moodOverride : selectedMood;
    const fullQuery = buildQuery(query, activeTag);
    if (fullQuery === lastQuery && mood === selectedMood && moodOverride === undefined) return;
    setSearchInput(query);
    setIsLoading(true);
    setError(null);

    try {
      const response = await searchGifs(fullQuery, TABLE_NAME, 20, mood ?? undefined);
      setGifs(response.results);
      setLastQuery(fullQuery);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      console.error('Search error:', err);
    } finally {
      setIsLoading(false);
    }
  }, [lastQuery, selectedMood, activeTag, buildQuery]);

  const handleClearSearch = useCallback(async () => {
    setDeepLinkPending(false); // unlock URL sync if still pending from failed load
    setSearchKey(k => k + 1);
    setLastQuery('');
    setSearchInput('');
    setActiveTag(null);
    setSelectedMood(null);
    setIsLoading(true);
    setError(null);
    try {
      const response = await getRandomGifs(TABLE_NAME, 30);
      setGifs(response.results);
      setTotalGifs(response.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load media items');
    } finally {
      setIsLoading(false);
    }
  }, []);

  const handleMoodSelect = useCallback(async (mood: MoodValue | null) => {
    setSelectedMood(mood);
    setIsLoading(true);
    setError(null);
    const fullQuery = buildQuery(searchInput, activeTag);

    try {
      if (mood && fullQuery) {
        const response = await searchGifs(fullQuery, TABLE_NAME, 20, mood);
        setGifs(response.results);
        setLastQuery(fullQuery);
      } else if (mood) {
        const response = await getRandomGifs(TABLE_NAME, 30, mood);
        setGifs(response.results);
        setLastQuery('');
      } else {
        if (fullQuery) {
          const response = await searchGifs(fullQuery, TABLE_NAME, 20);
          setGifs(response.results);
          setLastQuery(fullQuery);
        } else {
          const response = await getRandomGifs(TABLE_NAME, 30);
          setGifs(response.results);
          setTotalGifs(response.total);
          setLastQuery('');
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setIsLoading(false);
    }
  }, [searchInput, activeTag, buildQuery]);

  // Handle tag click from GifDetail: replace any current tag and search
  const handleTagClick = useCallback(async (tag: string) => {
    setActiveTag(tag);
    setSelectedGif(null); // close detail
    setSearchKey(k => k + 1); // force SearchBox remount with new value
    setLastQuery(''); // reset so handleSearch doesn't skip
    setIsLoading(true);
    setError(null);
    const fullQuery = buildQuery(searchInput, tag);
    try {
      const response = await searchGifs(fullQuery, TABLE_NAME, 20, selectedMood ?? undefined);
      setGifs(response.results);
      setLastQuery(fullQuery);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setIsLoading(false);
    }
  }, [searchInput, selectedMood, buildQuery]);

  // Handle removing the active tag bubble
  const handleRemoveTag = useCallback(async () => {
    setActiveTag(null);
    setLastQuery(''); // reset so next search fires
    setSearchKey(k => k + 1);
    if (searchInput.trim()) {
      // Re-search with just the text query
      setIsLoading(true);
      setError(null);
      try {
        const response = await searchGifs(searchInput.trim(), TABLE_NAME, 20, selectedMood ?? undefined);
        setGifs(response.results);
        setLastQuery(searchInput.trim());
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Search failed');
      } finally {
        setIsLoading(false);
      }
    } else {
      // No text either — show random
      setIsLoading(true);
      setError(null);
      try {
        const response = await getRandomGifs(TABLE_NAME, 30);
        setGifs(response.results);
        setTotalGifs(response.total);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load media items');
      } finally {
        setIsLoading(false);
      }
    }
  }, [searchInput, selectedMood]);

  // Handle mood emoji click from GifDetail
  const handleMoodClick = useCallback((mood: MoodValue) => {
    setSelectedGif(null);
    handleMoodSelect(mood);
  }, [handleMoodSelect]);

  return (
    <div className="min-h-screen bg-[hsl(var(--background))]">
      {/* Header */}
      <header className="sticky top-0 z-10 bg-[hsl(var(--background))]/95 backdrop-blur border-b border-[hsl(var(--border))]">
        <div className="max-w-7xl mx-auto px-4 py-4">
          <div className="flex items-center justify-between mb-4">
            <h1
              className="text-2xl font-bold text-[hsl(var(--foreground))] cursor-pointer hover:opacity-80 transition-opacity"
              onClick={handleClearSearch}
              title="Clear search"
            >
              MediaAF
            </h1>
            <button
              onClick={() => setIsDark(!isDark)}
              className="p-2 rounded-lg hover:bg-[hsl(var(--muted))] transition-colors"
              title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {isDark ? (
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <circle cx="12" cy="12" r="5" />
                  <line x1="12" y1="1" x2="12" y2="3" />
                  <line x1="12" y1="21" x2="12" y2="23" />
                  <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
                  <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
                  <line x1="1" y1="12" x2="3" y2="12" />
                  <line x1="21" y1="12" x2="23" y2="12" />
                  <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
                  <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
                </svg>
              ) : (
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
                </svg>
              )}
            </button>
          </div>
          <SearchBox key={searchKey} onSearch={handleSearch} isLoading={isLoading} initialValue={searchInput} activeTag={activeTag} onRemoveTag={handleRemoveTag} />
          <MoodFilterBar selected={selectedMood} onSelect={handleMoodSelect} />
        </div>
      </header>

      {/* Main content */}
      <main className="max-w-7xl mx-auto px-4 py-6">
        {error ? (
          <div className="text-center py-12">
            <p className="text-red-500 mb-2">{error}</p>
            <p className="text-[hsl(var(--muted-foreground))] text-sm">
              Make sure Antfly is running: <code className="bg-[hsl(var(--muted))] px-2 py-1 rounded">antfly swarm</code>
            </p>
          </div>
        ) : initialLoadDone && gifs.length === 0 && !lastQuery && !selectedMood ? (
          <div className="text-center py-12">
            <h2 className="text-xl font-semibold text-[hsl(var(--foreground))] mb-4">
              No media items loaded yet
            </h2>
            <p className="text-[hsl(var(--muted-foreground))] mb-4">
              You need to import the Tmedia dataset first:
            </p>
            <div className="bg-[hsl(var(--muted))] rounded-lg p-4 max-w-xl mx-auto text-left">
              <pre className="text-sm overflow-x-auto">
{`# 1. Make sure Antfly is running
antfly swarm

# 2. Import the dataset (from gif-picker/ingest)
cd ingest
go run main.go -tsv /path/to/Tmedia-Release/data/tgif-v1.0.tsv

# For a quick test, limit to 1000 media items:
go run main.go -tsv /path/to/Tmedia-Release/data/tgif-v1.0.tsv -limit 1000`}
              </pre>
            </div>
            <p className="text-[hsl(var(--muted-foreground))] mt-4 text-sm">
              Check the browser console for debugging info.
            </p>
          </div>
        ) : (
          <>
            {(lastQuery || selectedMood || (isLoading && searchInput)) && (
              <p className="text-[hsl(var(--muted-foreground))] mb-4">
                {isLoading && (searchInput || selectedMood)
                  ? `Searching${searchInput ? ` for "${searchInput}"` : ''}…`
                  : `${gifs.length} results${lastQuery ? ` for "${lastQuery}"` : ''}${selectedMood ? ` (${MOOD_EMOJIS.find(m => m.value === selectedMood)?.emoji ?? ''} ${selectedMood})` : ''}`
                }
              </p>
            )}
            <GifGrid gifs={gifs} isLoading={isLoading} onGifClick={setSelectedGif} hasActiveSearch={!!(lastQuery || selectedMood)} />
          </>
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-[hsl(var(--border))] mt-12">
        <div className="max-w-7xl mx-auto px-4 py-6 text-center text-[hsl(var(--muted-foreground))] text-sm">
          <p>
            Powered by{' '}
            <a
              href="https://antfly.io"
              className="text-[hsl(var(--foreground))] hover:underline"
              target="_blank"
              rel="noopener noreferrer"
            >
              Antfly
            </a>
            {totalGifs != null && totalGifs > 0 && (
              <> &bull; {totalGifs.toLocaleString()} media items</>
            )}
          </p>
          <p className="mt-1">
            <button
              onClick={() => setShowAbout(true)}
              className="text-[hsl(var(--foreground))] hover:underline"
            >
              About MediaAF
            </button>
          </p>
        </div>
      </footer>

      {/* Detail overlay */}
      {selectedGif && (
        <GifDetail gif={selectedGif} onClose={() => setSelectedGif(null)} hasActiveSearch={!!lastQuery} onTagClick={handleTagClick} onMoodClick={handleMoodClick} />
      )}

      {/* About modal */}
      {showAbout && <AboutModal onClose={() => setShowAbout(false)} />}
    </div>
  );
}

export default App;
