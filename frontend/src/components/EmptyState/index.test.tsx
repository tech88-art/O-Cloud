import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Button } from 'antd';
import { EmptyState } from './index';

describe('EmptyState', () => {
  it('renders the empty state container', () => {
    render(<EmptyState />);
    expect(screen.getByTestId('empty-state')).toBeInTheDocument();
  });

  it('renders custom description when provided', () => {
    render(<EmptyState description="No workloads yet" />);
    expect(screen.getByText('No workloads yet')).toBeInTheDocument();
  });

  it('renders provided action node', () => {
    render(
      <EmptyState
        description="No data"
        action={<Button data-testid="empty-cta">Deploy</Button>}
      />,
    );
    expect(screen.getByTestId('empty-cta')).toBeInTheDocument();
  });

  it('renders without action when none supplied', () => {
    render(<EmptyState description="Nothing here" />);
    expect(screen.queryByTestId('empty-cta')).not.toBeInTheDocument();
  });

  it('forwards custom className to root', () => {
    const { container } = render(
      <EmptyState description="x" className="custom-empty" />,
    );
    expect(container.querySelector('.custom-empty')).not.toBeNull();
  });
});
