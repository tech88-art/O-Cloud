import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ErrorState } from './index';

describe('ErrorState', () => {
  it('renders the alert container with the message string', () => {
    render(<ErrorState error="Backend unreachable" />);
    expect(screen.getByTestId('error-state')).toBeInTheDocument();
    expect(screen.getByText('Backend unreachable')).toBeInTheDocument();
  });

  it('extracts message from an Error instance', () => {
    render(<ErrorState error={new Error('NPU pool not found')} />);
    expect(screen.getByText('NPU pool not found')).toBeInTheDocument();
  });

  it('omits the Retry button when no onRetry callback is supplied', () => {
    render(<ErrorState error="x" />);
    expect(
      screen.queryByTestId('error-state-retry'),
    ).not.toBeInTheDocument();
  });

  it('renders the Retry button when onRetry is supplied', () => {
    render(<ErrorState error="x" onRetry={() => {}} />);
    expect(screen.getByTestId('error-state-retry')).toBeInTheDocument();
  });

  it('invokes onRetry when the retry button is clicked', async () => {
    const onRetry = vi.fn();
    render(<ErrorState error="x" onRetry={onRetry} />);
    await userEvent.click(screen.getByTestId('error-state-retry'));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('honours custom retryLabel and title', () => {
    render(
      <ErrorState
        error="bad"
        onRetry={() => {}}
        title="加载失败"
        retryLabel="重试"
      />,
    );
    expect(screen.getByText('加载失败')).toBeInTheDocument();
    expect(screen.getByText('重试')).toBeInTheDocument();
  });

  it('uses default title "Error" when not supplied', () => {
    render(<ErrorState error="x" />);
    expect(screen.getByText('Error')).toBeInTheDocument();
  });
});
