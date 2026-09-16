// Antfly API client for media search

const API_BASE = '/api/v1';

declare const __NSFW_MODE__: boolean;

// In SFW mode (default), hide media items with these tags or rating:X.
// Start with NSFW=1 npm run dev to disable all content filtering.
const BLOCKED_TAGS: Set<string> = __NSFW_MODE__
  ? new Set()
  : new Set(['porn', 'sexual', 'nsfw', 'adult-content']);
const BLOCKED_RATINGS: Set<string> = __NSFW_MODE__
  ? new Set()
  : new Set(['x']);

export interface GifResult {
  id: string;
  score: number;
  gif_url: string;
  description: string;
  tumblr_id?: string;
  original_description?: string;
  literal?: string;
  mood?: string;
  mood_emoji?: string;
  action?: string | string[];
  context?: string;
  source?: string;
  tags?: string[];
  attribution?: string;
  combined_text?: string;
  rank?: number;
  [key: string]: unknown; // allow arbitrary extra fields from API
}

export interface SearchResponse {
  results: GifResult[];
  total: number;
}

export async function getGifById(tableName: string, id: string): Promise<GifResult | null> {
  try {
    const response = await fetch(`${API_BASE}/tables/${tableName}/query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        full_text_search: { term: id, field: '_id' },
        limit: 1,
      }),
    });
    if (!response.ok) return null;
    const data = await response.json();
    const hit = data.responses?.[0]?.hits?.hits?.[0];
    if (!hit) return null;
    const source = hit._source ?? hit.source ?? {};
    if (isRemovedGif(source) || hasBlockedTag(source) || hasBlockedRating(source)) return null;
    return {
      ...source,
      id: hit._id ?? hit.id ?? id,
      score: 0,
      gif_url: source.gif_url ?? '',
      description: source.description ?? source.original_description ?? '',
    };
  } catch {
    return null;
  }
}

// Detect media items removed by Tumblr (copyright/guideline violations)
function isRemovedGif(source: Record<string, any>): boolean {
  const fields = [source.literal, source.description, source.combined_text];
  return fields.some(f => typeof f === 'string' && f.toLowerCase().includes('content has been removed'));
}

// Check if media has any blocked tags
function hasBlockedTag(source: Record<string, any>): boolean {
  const tags = source.tags;
  if (!Array.isArray(tags)) return false;
  return tags.some(tag => typeof tag === 'string' && BLOCKED_TAGS.has(tag.toLowerCase()));
}

// Check if media has a blocked rating
function hasBlockedRating(source: Record<string, any>): boolean {
  const rating = source.rating;
  if (typeof rating !== 'string') return false;
  return BLOCKED_RATINGS.has(rating.toLowerCase());
}

// Google-style query parsing: "quoted phrases", tag:X, -tag:X, and loose terms
interface ParsedQuery {
  phrases: string[];    // "quoted phrases" → match_phrase
  looseText: string;    // unquoted terms → match + semantic
  tags: string[];
  negativeTags: string[];
  ratings: string[];
}

function parseQuery(raw: string): ParsedQuery {
  const tags: string[] = [];
  const negativeTags: string[] = [];
  const ratings: string[] = [];
  const phrases: string[] = [];

  // Strip -tag:"quoted" and -tag:word first
  let remaining = raw
    .replace(/-tag:"([^"]+)"/g, (_, tag) => {
      negativeTags.push(tag.toLowerCase());
      return '';
    })
    .replace(/-tag:(\S+)/g, (_, tag) => {
      negativeTags.push(tag.toLowerCase());
      return '';
    })
    // Then tag:"quoted" and tag:word
    .replace(/tag:"([^"]+)"/g, (_, tag) => {
      tags.push(tag.toLowerCase());
      return '';
    })
    .replace(/tag:(\S+)/g, (_, tag) => {
      tags.push(tag.toLowerCase());
      return '';
    })
    // rating:"quoted" and rating:word
    .replace(/rating:"([^"]+)"/g, (_, r) => {
      ratings.push(r.toLowerCase());
      return '';
    })
    .replace(/rating:(\S+)/g, (_, r) => {
      ratings.push(r.toLowerCase());
      return '';
    });

  // Extract "quoted phrases"
  remaining = remaining.replace(/"([^"]+)"/g, (_, phrase) => {
    phrases.push(phrase.trim());
    return '';
  });

  const looseText = remaining.trim();
  return { phrases, looseText, tags, negativeTags, ratings };
}

// Build the full_text_search value from parsed query components
function buildFullTextSearch(phrases: string[], looseText: string): Record<string, unknown> | undefined {
  const parts: Record<string, unknown>[] = [];

  for (const phrase of phrases) {
    parts.push({ match_phrase: phrase, field: 'combined_text' });
  }
  if (looseText) {
    parts.push({ match: looseText, field: 'combined_text' });
  }

  if (parts.length === 0) return undefined;
  if (parts.length === 1) return parts[0];
  return { conjuncts: parts };
}

// Build exclusion_query from negative tags, blocked tags, and blocked ratings
function buildExclusionQuery(negativeTags: string[]): Record<string, unknown> | undefined {
  const parts: string[] = [];

  // Add globally blocked tags
  for (const tag of BLOCKED_TAGS) {
    parts.push(`tags:"${tag}"`);
  }

  // Add globally blocked ratings
  for (const rating of BLOCKED_RATINGS) {
    parts.push(`rating:"${rating}"`);
  }

  // Add user's negative tags
  for (const tag of negativeTags) {
    // Quote values to handle multi-word tags like "Live Leak"
    parts.push(`tags:"${tag}"`);
  }

  if (parts.length === 0) return undefined;
  return { query: parts.join(' OR ') };
}

export async function searchGifs(
  query: string,
  tableName: string,
  limit: number = 50,
  moodFilter?: string,
): Promise<SearchResponse> {
  const body: Record<string, unknown> = { limit };
  const { phrases, looseText, tags, negativeTags, ratings } = parseQuery(query);

  const fts = buildFullTextSearch(phrases, '');
  const hasTextSearch = !!(fts || looseText);

  if (fts) {
    body.full_text_search = fts;
  }

  // Only include semantic search when there are unquoted terms
  if (looseText) {
    body.semantic_search = looseText;
    body.indexes = ['embeddings'];
    if (fts) {
      body.merge_strategy = 'rrf';
    }
  }

  // Build structured filter queries from tag: and rating: prefixes
  const filterParts: Record<string, unknown>[] = [];

  // tag: filters — use match_phrase for compound tags since Bleve tokenizes
  // on hyphens and spaces (e.g. "adult-content" → ["adult","content"])
  for (const t of tags) {
    const bleveForm = t.replace(/-/g, ' ');
    filterParts.push(
      bleveForm.includes(' ')
        ? { match_phrase: bleveForm, field: 'tags' }
        : { term: bleveForm, field: 'tags' }
    );
  }

  // rating: filters — use match_phrase with hyphens converted to spaces
  // since Bleve tokenizes "PG-13" as ["pg","13"]
  for (const r of ratings) {
    const bleveForm = r.replace(/-/g, ' ');
    filterParts.push(
      bleveForm.includes(' ')
        ? { match_phrase: bleveForm, field: 'rating' }
        : { term: bleveForm, field: 'rating' }
    );
  }

  // Mood emoji filter
  if (moodFilter) {
    filterParts.push({ term: moodFilter, field: 'mood_emoji' });
  }

  if (filterParts.length > 0) {
    const filterQuery = filterParts.length === 1 ? filterParts[0] : { conjuncts: filterParts };

    if (hasTextSearch) {
      body.filter_query = filterQuery;
    } else {
      // Filter-only (no text/semantic search): use as primary full-text search
      body.full_text_search = filterQuery;
    }
  }

  // Apply negative tags exclusion
  const exclusion = buildExclusionQuery(negativeTags);
  if (exclusion) {
    body.exclusion_query = exclusion;
  }

  const response = await fetch(`${API_BASE}/tables/${tableName}/query`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`Search failed: ${response.statusText}`);
  }

  const data = await response.json();

  // Debug: log raw response in development
  if (import.meta.env.DEV) {
    console.log('Antfly raw response:', JSON.stringify(data, null, 2).slice(0, 1000));
  }

  // Check for error in response
  const firstResponse = data.responses?.[0];
  if (firstResponse?.error) {
    throw new Error(firstResponse.error);
  }

  // Transform Antfly response to our format
  const hits = firstResponse?.hits?.hits ?? [];
  const results: GifResult[] = hits
    .filter((hit: any) => {
      const source = hit.source ?? hit._source ?? {};
      return !isRemovedGif(source) && !hasBlockedTag(source) && !hasBlockedRating(source);
    })
    .map((hit: any, index: number) => {
      // Debug: log first hit structure
      if (import.meta.env.DEV && index === 0) {
        console.log('First hit structure:', hit);
      }
      const source = hit.source ?? hit._source ?? {};
      return {
        ...source,
        id: hit.id ?? hit._id ?? '',
        score: hit._index_scores?.embeddings ?? hit._score ?? 0,
        gif_url: source.gif_url ?? '',
        description: source.description ?? source.original_description ?? '',
        rank: index + 1,
      };
    });

  // Post-filter for exact tag/rating matching — Bleve tokenizes on hyphens,
  // so a term query for a tag also matches compound tags containing that word.
  // The server query is best-effort; we do exact string matching here.
  const filtered = results.filter((gif) => {
    if (tags.length > 0) {
      const gifTags = (gif.tags ?? []).map((t: string) => t.toLowerCase());
      if (!tags.every(t => gifTags.includes(t))) return false;
    }
    if (ratings.length > 0) {
      const gifRating = (typeof gif.rating === 'string' ? gif.rating : '').toLowerCase();
      if (!ratings.includes(gifRating)) return false;
    }
    return true;
  });

  return {
    results: filtered,
    total: firstResponse?.hits?.total ?? filtered.length,
  };
}

const TOTAL_CACHE_KEY = 'mediaaf_total';

// Max offset for random media loading — Bleve implements offset as a linear
// scan, so large values (e.g. 30k on 106k records) take ~29s and pin a CPU
// core. Capping at 2000 keeps scans under ~1s.
const MAX_RANDOM_OFFSET = 2000;

export async function getRandomGifs(tableName: string, limit: number = 30, moodFilter?: string): Promise<SearchResponse> {
  // When mood filter is active, use searchGifs with empty query to get filtered results
  if (moodFilter) {
    return searchGifs('', tableName, limit, moodFilter);
  }

  const exclusion = buildExclusionQuery([]);

  // Use cached total from localStorage to pick a random offset (0 on first visit)
  const cachedTotal = parseInt(localStorage.getItem(TOTAL_CACHE_KEY) ?? '0', 10);
  const maxOffset = Math.min(cachedTotal > limit ? cachedTotal - limit : 0, MAX_RANDOM_OFFSET);
  const randomOffset = maxOffset > 0 ? Math.floor(Math.random() * maxOffset) : 0;

  const body: Record<string, unknown> = { limit, offset: randomOffset };
  if (exclusion) body.exclusion_query = exclusion;

  const resp = await fetch(`${API_BASE}/tables/${tableName}/query`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!resp.ok) throw new Error(`Failed to load media items: ${resp.statusText}`);

  const data = await resp.json();
  const firstResponse = data.responses?.[0];
  const total = firstResponse?.hits?.total ?? 0;
  const hits = firstResponse?.hits?.hits ?? [];

  // Cache total for future offset calculations and footer display
  if (total > 0) {
    localStorage.setItem(TOTAL_CACHE_KEY, String(total));
  }

  // If stale cache caused offset to overshoot (0 results), retry once at offset 0
  if (hits.length === 0 && randomOffset > 0) {
    const retryBody: Record<string, unknown> = { limit, offset: 0 };
    if (exclusion) retryBody.exclusion_query = exclusion;

    const retryResp = await fetch(`${API_BASE}/tables/${tableName}/query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(retryBody),
    });
    if (!retryResp.ok) throw new Error(`Failed to load media items: ${retryResp.statusText}`);

    const retryData = await retryResp.json();
    const retryFirst = retryData.responses?.[0];
    const retryTotal = retryFirst?.hits?.total ?? 0;
    if (retryTotal > 0) {
      localStorage.setItem(TOTAL_CACHE_KEY, String(retryTotal));
    }
    return buildRandomResponse(retryFirst?.hits?.hits ?? [], retryTotal, limit);
  }

  return buildRandomResponse(hits, total, limit);
}

function buildRandomResponse(hits: any[], total: number, limit: number): SearchResponse {
  const pool: GifResult[] = hits
    .filter((hit: any) => {
      const source = hit.source ?? hit._source ?? {};
      return !isRemovedGif(source) && !hasBlockedTag(source) && !hasBlockedRating(source);
    })
    .map((hit: any) => {
      const source = hit.source ?? hit._source ?? {};
      return {
        ...source,
        id: hit.id ?? hit._id ?? '',
        score: hit._score ?? 1,
        gif_url: source.gif_url ?? '',
        description: source.description ?? source.original_description ?? '',
      };
    });

  // Fisher-Yates shuffle
  for (let i = pool.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }

  return {
    results: pool.slice(0, limit),
    total,
  };
}
