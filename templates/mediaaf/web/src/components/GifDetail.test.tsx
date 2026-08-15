import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { GifDetail } from './GifDetail';
import type { GifResult } from '../lib/antfly';

const mockGif: GifResult = {
  id: 'test-gif-1',
  score: 0.95,
  gif_url: 'https://example.com/test.gif',
  description: 'A test gif showing something cool',
  tumblr_id: 'tumblr_abc123',
  tags: ['funny', 'cat', 'dance'],
  literal: 'A cat dancing on a table',
  source: 'tumblr',
  mood: 'playful',
  rating: 'safe',
};

describe('GifDetail', () => {
  const mockOnClose = vi.fn();
  const mockOnTagClick = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render media image and metadata', () => {
    render(<GifDetail gif={mockGif} onClose={mockOnClose} />);

    const img = screen.getByRole('img');
    expect(img).toHaveAttribute('src', mockGif.gif_url);
    expect(img).toHaveAttribute('alt', mockGif.description);
    expect(screen.getByText(mockGif.gif_url)).toBeInTheDocument();
  });

  it('should render Copy URL button', () => {
    render(<GifDetail gif={mockGif} onClose={mockOnClose} />);

    expect(screen.getByText('Copy URL')).toBeInTheDocument();
  });

  it('should call onClose when Escape is pressed', () => {
    render(<GifDetail gif={mockGif} onClose={mockOnClose} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('should call onClose when backdrop is clicked', () => {
    render(<GifDetail gif={mockGif} onClose={mockOnClose} />);

    // Click the outermost overlay div (backdrop area)
    const backdrop = screen.getByRole('img').closest('.fixed');
    if (backdrop) fireEvent.click(backdrop);
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('should render tags as clickable buttons', () => {
    render(
      <GifDetail gif={mockGif} onClose={mockOnClose} onTagClick={mockOnTagClick} />,
    );

    const tagButtons = screen.getAllByRole('button').filter(
      (btn) => btn.getAttribute('title')?.startsWith('Search for tag:'),
    );
    expect(tagButtons).toHaveLength(3);

    fireEvent.click(tagButtons[0]);
    expect(mockOnTagClick).toHaveBeenCalledWith('funny');
  });
});
