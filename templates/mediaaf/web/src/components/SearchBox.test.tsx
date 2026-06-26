import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SearchBox } from './SearchBox';

describe('SearchBox', () => {
  let onSearch: ReturnType<typeof vi.fn<(query: string) => void>>;

  beforeEach(() => {
    onSearch = vi.fn<(query: string) => void>();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should render search input', () => {
    render(<SearchBox onSearch={onSearch} />);

    const input = screen.getByPlaceholderText(/search for media/i);
    expect(input).toBeInTheDocument();
  });

  it('should debounce search calls', async () => {
    render(<SearchBox onSearch={onSearch} />);

    const input = screen.getByPlaceholderText(/search for media/i);

    // Type quickly - wrap in act to properly handle state updates
    await act(async () => {
      fireEvent.change(input, { target: { value: 'c' } });
      fireEvent.change(input, { target: { value: 'ca' } });
      fireEvent.change(input, { target: { value: 'cat' } });
    });

    // Should not have called yet (before debounce time)
    expect(onSearch).not.toHaveBeenCalled();

    // Advance past debounce and flush pending effects
    await act(async () => {
      await vi.advanceTimersByTimeAsync(350);
    });

    // Should call once with final value
    expect(onSearch).toHaveBeenCalledTimes(1);
    expect(onSearch).toHaveBeenCalledWith('cat');
  });

  it('should not search on empty input', async () => {
    render(<SearchBox onSearch={onSearch} />);

    const input = screen.getByPlaceholderText(/search for media/i);

    await act(async () => {
      fireEvent.change(input, { target: { value: '   ' } });
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(350);
    });

    expect(onSearch).not.toHaveBeenCalled();
  });

  it('should trigger search on form submit', async () => {
    vi.useRealTimers(); // Need real timers for userEvent
    const user = userEvent.setup();

    render(<SearchBox onSearch={onSearch} />);

    const input = screen.getByPlaceholderText(/search for media/i);

    await user.type(input, 'dog');
    await user.keyboard('{Enter}');

    await waitFor(() => {
      expect(onSearch).toHaveBeenCalledWith('dog');
    });
  });

  it('should show loading spinner when isLoading is true', () => {
    render(<SearchBox onSearch={onSearch} isLoading={true} />);

    // The spinner is a div with animate-spin class
    const spinner = document.querySelector('.animate-spin');
    expect(spinner).toBeInTheDocument();
  });

  it('should not show loading spinner when isLoading is false', () => {
    render(<SearchBox onSearch={onSearch} isLoading={false} />);

    const spinner = document.querySelector('.animate-spin');
    expect(spinner).not.toBeInTheDocument();
  });

  it('should autofocus the input', () => {
    render(<SearchBox onSearch={onSearch} />);

    const input = screen.getByPlaceholderText(/search for media/i);
    expect(input).toHaveFocus();
  });
});
