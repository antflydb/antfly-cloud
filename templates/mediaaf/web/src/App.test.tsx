import { StrictMode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';
import * as antfly from './lib/antfly';

vi.mock('./lib/antfly', () => ({
  searchGifs: vi.fn(),
  getRandomGifs: vi.fn(),
  getGifById: vi.fn(),
}));

const mockRandomResponse: antfly.SearchResponse = {
  results: [
    { id: 'random_1', score: 1, gif_url: 'https://example.com/1.gif', description: 'Random media 1' },
    { id: 'random_2', score: 1, gif_url: 'https://example.com/2.gif', description: 'Random media 2' },
  ],
  total: 100,
};

const mockDeepLinkedGif: antfly.GifResult = {
  id: 'sources_2a7308db',
  score: 0,
  gif_url: 'https://example.com/deep-linked.gif',
  description: 'A deep-linked GIF',
};

/** Resolve after a few microtask ticks — simulates async without real delay */
function tick<T>(value: T): Promise<T> {
  return Promise.resolve().then(() => value);
}

// All tests render inside StrictMode to match main.tsx (double-invokes effects)
function renderApp() {
  return render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}

describe('App deep link (?gif=ID) under StrictMode', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    window.history.replaceState(null, '', '/');
  });

  it('should open media detail when ?gif=ID is in URL and media is fetched by ID', async () => {
    window.history.replaceState(null, '', '/?gif=sources_2a7308db');

    vi.mocked(antfly.getRandomGifs).mockImplementation(() => tick(mockRandomResponse));
    vi.mocked(antfly.getGifById).mockImplementation(() => tick(mockDeepLinkedGif));

    renderApp();

    await waitFor(() => {
      expect(screen.getByText('Copy URL')).toBeInTheDocument();
    });

    expect(window.location.search).toContain('gif=sources_2a7308db');
  });

  it('should open media detail when deep-linked media is found in random results', async () => {
    window.history.replaceState(null, '', '/?gif=random_1');

    vi.mocked(antfly.getRandomGifs).mockImplementation(() => tick(mockRandomResponse));

    renderApp();

    await waitFor(() => {
      expect(screen.getByText('Copy URL')).toBeInTheDocument();
    });

    expect(antfly.getGifById).not.toHaveBeenCalled();
    expect(window.location.search).toContain('gif=random_1');
  });

  it('should not show detail pane when deep-linked media is not found', async () => {
    window.history.replaceState(null, '', '/?gif=nonexistent_id');

    vi.mocked(antfly.getRandomGifs).mockImplementation(() => tick(mockRandomResponse));
    vi.mocked(antfly.getGifById).mockImplementation(() => tick(null));

    renderApp();

    await waitFor(() => {
      expect(antfly.getGifById).toHaveBeenCalledWith('honeycomb', 'nonexistent_id');
    });

    expect(screen.queryByText('Copy URL')).not.toBeInTheDocument();
    expect(window.location.search).not.toContain('gif=');
  });

  it('should not strip ?gif= param while deep link fetch is still pending', async () => {
    window.history.replaceState(null, '', '/?gif=sources_2a7308db');

    vi.mocked(antfly.getRandomGifs).mockImplementation(() => tick(mockRandomResponse));

    // getGifById hangs until we manually resolve it
    let resolveGifById!: (value: antfly.GifResult | null) => void;
    vi.mocked(antfly.getGifById).mockImplementation(
      () => new Promise((resolve) => { resolveGifById = resolve; }),
    );

    renderApp();

    // Wait for getRandomGifs to complete and getGifById to be called
    await waitFor(() => {
      expect(antfly.getGifById).toHaveBeenCalled();
    });

    // URL must NOT be cleared while getGifById is still pending
    expect(window.location.search).toContain('gif=sources_2a7308db');
    expect(screen.queryByText('Copy URL')).not.toBeInTheDocument();

    // Now resolve the deep link
    resolveGifById(mockDeepLinkedGif);

    await waitFor(() => {
      expect(screen.getByText('Copy URL')).toBeInTheDocument();
    });
    expect(window.location.search).toContain('gif=sources_2a7308db');
  });

  it('should not open detail pane when there is no ?gif= param', async () => {
    window.history.replaceState(null, '', '/');

    vi.mocked(antfly.getRandomGifs).mockImplementation(() => tick(mockRandomResponse));

    renderApp();

    await waitFor(() => {
      expect(antfly.getRandomGifs).toHaveBeenCalled();
    });

    expect(screen.queryByText('Copy URL')).not.toBeInTheDocument();
    expect(window.location.search).not.toContain('gif=');
    expect(antfly.getGifById).not.toHaveBeenCalled();
  });

  it('should preserve ?gif= in URL when getRandomGifs fails', async () => {
    window.history.replaceState(null, '', '/?gif=sources_2a7308db');

    vi.mocked(antfly.getRandomGifs).mockImplementation(
      () => Promise.reject(new Error('Network error')),
    );

    renderApp();

    await waitFor(() => {
      expect(screen.getByText(/Network error/)).toBeInTheDocument();
    });

    // URL should still have ?gif= so user can retry with a refresh
    expect(window.location.search).toContain('gif=sources_2a7308db');
  });
});
