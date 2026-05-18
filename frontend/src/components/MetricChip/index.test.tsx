import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MetricChip } from './index';

describe('MetricChip', () => {
  it('renders required label + value', () => {
    render(<MetricChip label="Avg AI Core Util" value={65} />);
    const chip = screen.getByTestId('metric-chip');
    expect(chip).toBeInTheDocument();
    expect(chip.textContent).toContain('Avg AI Core Util');
    expect(chip.textContent).toContain('65');
  });

  it('renders unit suffix when provided', () => {
    render(<MetricChip label="Util" value={65} unit="%" />);
    expect(screen.getByTestId('metric-chip').textContent).toContain('%');
  });

  it('renders trend indicator when trend prop set', () => {
    render(<MetricChip label="Util" value={65} trend="up" />);
    const trend = screen.getByTestId('metric-chip-trend');
    expect(trend).toBeInTheDocument();
    expect(trend.textContent).toBe('↑');
    expect(trend.getAttribute('aria-label')).toBe('trend up');
  });

  it('omits trend element when no trend prop', () => {
    render(<MetricChip label="Util" value={65} />);
    expect(screen.queryByTestId('metric-chip-trend')).not.toBeInTheDocument();
    expect(screen.getByTestId('metric-chip').getAttribute('data-trend')).toBe(
      'none',
    );
  });

  it('renders down trend glyph correctly', () => {
    render(<MetricChip label="Latency" value={50} trend="down" />);
    expect(screen.getByTestId('metric-chip-trend').textContent).toBe('↓');
  });

  it('renders flat trend glyph correctly', () => {
    render(<MetricChip label="Pods" value={12} trend="flat" />);
    expect(screen.getByTestId('metric-chip-trend').textContent).toBe('→');
  });

  it('applies tone attribute when supplied', () => {
    render(<MetricChip label="Util" value={95} tone="danger" />);
    expect(screen.getByTestId('metric-chip').getAttribute('data-tone')).toBe(
      'danger',
    );
  });

  it('defaults tone to "default"', () => {
    render(<MetricChip label="x" value={1} />);
    expect(screen.getByTestId('metric-chip').getAttribute('data-tone')).toBe(
      'default',
    );
  });

  it('accepts ReactNode values (e.g. JSX)', () => {
    render(
      <MetricChip
        label={<strong>Custom Label</strong>}
        value={<em>Custom Value</em>}
      />,
    );
    expect(screen.getByText('Custom Label')).toBeInTheDocument();
    expect(screen.getByText('Custom Value')).toBeInTheDocument();
  });
});
