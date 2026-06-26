import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GifGrid } from './GifGrid';
import type { GifResult } from '../lib/antfly';

const mockGifs: GifResult[] = [
  {
    id: 'gif-1',
    score: 0.95,
    gif_url: 'https://example.com/1.gif',
    description: 'First gif',
    tumblr_id: 'tumblr_1',
  },
  {
    id: 'gif-2',
    score: 0.85,
    gif_url: 'https://example.com/2.gif',
    description: 'Second gif',
    tumblr_id: 'tumblr_2',
  },
  {
    id: 'gif-3',
    score: 0.75,
    gif_url: 'https://example.com/3.gif',
    description: 'Third gif',
    tumblr_id: 'tumblr_3',
  },
];

describe('GifGrid', () => {
  it('should render loading skeleton when loading with no gifs', () => {
    render(<GifGrid gifs={[]} isLoading={true} />);

    // Should show 20 skeleton items
    const skeletons = document.querySelectorAll('.animate-pulse');
    expect(skeletons).toHaveLength(20);
  });

  it('should render empty state when no gifs and not loading', () => {
    render(<GifGrid gifs={[]} isLoading={false} />);

    expect(screen.getByText('No media items found')).toBeInTheDocument();
    expect(screen.getByText('Try a different search term')).toBeInTheDocument();
  });

  it('should render all gif cards', () => {
    render(<GifGrid gifs={mockGifs} />);

    // Each gif should have its description as alt text
    expect(screen.getByAltText('First gif')).toBeInTheDocument();
    expect(screen.getByAltText('Second gif')).toBeInTheDocument();
    expect(screen.getByAltText('Third gif')).toBeInTheDocument();
  });

  it('should render gifs even while loading', () => {
    render(<GifGrid gifs={mockGifs} isLoading={true} />);

    // Should show gifs, not skeleton
    expect(screen.getByAltText('First gif')).toBeInTheDocument();
    expect(document.querySelectorAll('.animate-pulse')).toHaveLength(3); // Just card skeletons
  });

  it('should use responsive masonry columns layout', () => {
    render(<GifGrid gifs={mockGifs} />);

    const container = document.querySelector('.columns-2');
    expect(container).toBeInTheDocument();
    expect(container).toHaveClass('sm:columns-3');
    expect(container).toHaveClass('md:columns-4');
    expect(container).toHaveClass('lg:columns-5');
  });

  it('should handle gifs with missing IDs by using index', () => {
    const gifsWithMissingIds: GifResult[] = [
      { ...mockGifs[0], id: '' },
      { ...mockGifs[1], id: '' },
    ];

    // Should not throw or warn about duplicate keys
    const { container } = render(<GifGrid gifs={gifsWithMissingIds} />);

    // Both cards should render
    const cards = container.querySelectorAll('.group');
    expect(cards).toHaveLength(2);
  });

  it('should handle single gif', () => {
    render(<GifGrid gifs={[mockGifs[0]]} />);

    expect(screen.getByAltText('First gif')).toBeInTheDocument();
  });

  it('should handle large number of gifs', () => {
    const manyGifs = Array.from({ length: 100 }, (_, i) => ({
      id: `gif-${i}`,
      score: 0.5,
      gif_url: `https://example.com/${i}.gif`,
      description: `Gif number ${i}`,
      tumblr_id: `tumblr_${i}`,
    }));

    render(<GifGrid gifs={manyGifs} />);

    // Check a few rendered (descriptions are in alt text)
    expect(screen.getByAltText('Gif number 0')).toBeInTheDocument();
    expect(screen.getByAltText('Gif number 50')).toBeInTheDocument();
    expect(screen.getByAltText('Gif number 99')).toBeInTheDocument();
  });
});
