import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Button } from 'antd';
import { ResourceCard } from './index';

describe('ResourceCard', () => {
  it('renders required title and children', () => {
    render(
      <ResourceCard title="cluster-a">
        <p>4 nodes, 16 NPUs</p>
      </ResourceCard>,
    );
    expect(screen.getByTestId('resource-card')).toBeInTheDocument();
    expect(screen.getByText('cluster-a')).toBeInTheDocument();
    expect(screen.getByText('4 nodes, 16 NPUs')).toBeInTheDocument();
  });

  it('renders description when provided', () => {
    render(
      <ResourceCard title="node-1" description="Ascend 910B x4">
        body
      </ResourceCard>,
    );
    expect(
      screen.getByTestId('resource-card-description'),
    ).toHaveTextContent('Ascend 910B x4');
  });

  it('omits the description element when not provided', () => {
    render(<ResourceCard title="node-1">body</ResourceCard>);
    expect(
      screen.queryByTestId('resource-card-description'),
    ).not.toBeInTheDocument();
  });

  it('renders an embedded StatusTag when status is provided', () => {
    render(
      <ResourceCard title="cluster-a" status="healthy">
        body
      </ResourceCard>,
    );
    const tag = screen.getByTestId('status-tag');
    expect(tag).toBeInTheDocument();
    expect(tag.getAttribute('data-tone')).toBe('success');
  });

  it('renders action node(s) in the extra slot', () => {
    render(
      <ResourceCard
        title="cluster-a"
        actions={<Button data-testid="card-cta">Deploy</Button>}
      >
        body
      </ResourceCard>,
    );
    expect(screen.getByTestId('card-cta')).toBeInTheDocument();
  });

  it('renders status + actions together when both supplied', () => {
    render(
      <ResourceCard
        title="cluster-a"
        status="degraded"
        actions={<Button data-testid="card-cta">Reconcile</Button>}
      >
        body
      </ResourceCard>,
    );
    expect(screen.getByTestId('status-tag')).toBeInTheDocument();
    expect(screen.getByTestId('card-cta')).toBeInTheDocument();
  });

  it('honours custom testId', () => {
    render(
      <ResourceCard title="x" testId="custom-rc">
        body
      </ResourceCard>,
    );
    expect(screen.getByTestId('custom-rc')).toBeInTheDocument();
  });
});
