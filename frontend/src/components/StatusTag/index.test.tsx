import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatusTag, DEFAULT_STATUS_MAP } from './index';

describe('StatusTag', () => {
  it('renders with the raw status text by default', () => {
    render(<StatusTag status="healthy" />);
    const tag = screen.getByTestId('status-tag');
    expect(tag).toBeInTheDocument();
    expect(tag.textContent).toBe('healthy');
  });

  it('maps "healthy" to success tone (green)', () => {
    render(<StatusTag status="healthy" />);
    expect(screen.getByTestId('status-tag').getAttribute('data-tone')).toBe(
      'success',
    );
  });

  it('is case-insensitive: "READY" maps the same as "ready"', () => {
    render(<StatusTag status="READY" />);
    expect(screen.getByTestId('status-tag').getAttribute('data-tone')).toBe(
      'success',
    );
  });

  it('handles whitespace/underscore/dash variants ("Not Ready" === "NotReady")', () => {
    render(<StatusTag status="Not Ready" />);
    expect(screen.getByTestId('status-tag').getAttribute('data-tone')).toBe(
      'error',
    );
  });

  it('falls back to default tone for unknown status', () => {
    render(<StatusTag status="🦄unicorn-state" />);
    expect(screen.getByTestId('status-tag').getAttribute('data-tone')).toBe(
      'default',
    );
  });

  it('allows mapping override to take precedence over the built-in map', () => {
    render(
      <StatusTag
        status="healthy"
        mapping={{ healthy: 'error' }}
      />,
    );
    expect(screen.getByTestId('status-tag').getAttribute('data-tone')).toBe(
      'error',
    );
  });

  it('renders a custom label when provided', () => {
    render(<StatusTag status="running" label="正在运行" />);
    expect(screen.getByTestId('status-tag').textContent).toBe('正在运行');
  });

  it('built-in map covers at least 10 common statuses', () => {
    // P1-T-107 AC: "StatusTag color map covers ≥ 10 common statuses"
    const required = [
      'healthy',
      'degraded',
      'faulty',
      'offline',
      'ready',
      'notready',
      'unknown',
      'running',
      'pending',
      'succeeded',
      'failed',
    ];
    for (const k of required) {
      expect(DEFAULT_STATUS_MAP[k]).toBeDefined();
    }
    expect(Object.keys(DEFAULT_STATUS_MAP).length).toBeGreaterThanOrEqual(10);
  });
});
