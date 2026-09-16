import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { searchGifs, getGifById } from './antfly';

// Mock fetch globally
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

// Mock localStorage (jsdom's implementation is incomplete)
const store: Record<string, string> = {};
const mockLocalStorage = {
  getItem: vi.fn((key: string) => store[key] ?? null),
  setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
  removeItem: vi.fn((key: string) => { delete store[key]; }),
  clear: vi.fn(() => { for (const key in store) delete store[key]; }),
  get length() { return Object.keys(store).length; },
  key: vi.fn((i: number) => Object.keys(store)[i] ?? null),
};
Object.defineProperty(globalThis, 'localStorage', { value: mockLocalStorage, writable: true });

const TEXT_TABLE = 'mediaaf';

describe('Antfly API Client', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('searchGifs', () => {
    it('should send correct request to Antfly API', async () => {
      const mockResponse = {
        responses: [
          {
            hits: {
              hits: [
                {
                  id: 'gif_123',
                  _index_scores: { embeddings: 0.95 },
                  source: {
                    gif_url: 'https://example.com/cat.gif',
                    description: 'a cat playing',
                    tumblr_id: 'abc123',
                  },
                },
              ],
              total: 1,
            },
          },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await searchGifs('cat playing', TEXT_TABLE, 50);

      // Verify the request
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/v1/tables/mediaaf/query',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
        })
      );

      // Verify the response transformation
      expect(result.results).toHaveLength(1);
      expect(result.results[0].id).toBe('gif_123');
      expect(result.results[0].gif_url).toBe('https://example.com/cat.gif');
      expect(result.total).toBe(1);
    });

    it('should handle empty results', async () => {
      const mockResponse = {
        responses: [
          {
            hits: {
              hits: [],
              total: 0,
            },
          },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await searchGifs('nonexistent query', TEXT_TABLE);

      expect(result.results).toHaveLength(0);
      expect(result.total).toBe(0);
    });

    it('should handle missing fields gracefully', async () => {
      const mockResponse = {
        responses: [
          {
            hits: {
              hits: [
                {
                  id: 'gif_456',
                  _score: 0.8,
                  source: {}, // Missing fields
                },
              ],
            },
          },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await searchGifs('test', TEXT_TABLE);

      expect(result.results[0].id).toBe('gif_456');
      expect(result.results[0].gif_url).toBe('');
      expect(result.results[0].description).toBe('');
    });

    it('should handle _source format (Elasticsearch style)', async () => {
      const mockResponse = {
        responses: [
          {
            hits: {
              hits: [
                {
                  _id: 'gif_789',
                  _score: 0.9,
                  _source: {
                    gif_url: 'https://example.com/es.gif',
                    description: 'elasticsearch style',
                    tumblr_id: 'es123',
                  },
                },
              ],
              total: 1,
            },
          },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await searchGifs('test', TEXT_TABLE);

      expect(result.results[0].id).toBe('gif_789');
      expect(result.results[0].gif_url).toBe('https://example.com/es.gif');
    });

    it('should throw error on API failure', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        statusText: 'Internal Server Error',
      });

      await expect(searchGifs('test', TEXT_TABLE)).rejects.toThrow('Search failed: Internal Server Error');
    });

    it('should handle malformed response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({}), // Empty response
      });

      const result = await searchGifs('test', TEXT_TABLE);

      expect(result.results).toHaveLength(0);
    });
  });

  describe('getGifById', () => {
    it('should query by _id and return the GIF', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          responses: [{
            hits: {
              hits: [{
                _id: 'sources_abc123',
                _source: {
                  gif_url: 'https://example.com/found.gif',
                  description: 'a found gif',
                },
              }],
              total: 1,
            },
          }],
        }),
      });

      const result = await getGifById(TEXT_TABLE, 'sources_abc123');

      expect(mockFetch).toHaveBeenCalledWith(
        '/api/v1/tables/mediaaf/query',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            full_text_search: { term: 'sources_abc123', field: '_id' },
            limit: 1,
          }),
        }),
      );

      expect(result).not.toBeNull();
      expect(result!.id).toBe('sources_abc123');
      expect(result!.gif_url).toBe('https://example.com/found.gif');
    });

    it('should return null when no results', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          responses: [{ hits: { hits: [], total: 0 } }],
        }),
      });

      const result = await getGifById(TEXT_TABLE, 'nonexistent');
      expect(result).toBeNull();
    });

    it('should return null on fetch error', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'));

      const result = await getGifById(TEXT_TABLE, 'any_id');
      expect(result).toBeNull();
    });
  });

});
