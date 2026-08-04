import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { RunStatus } from '@/components/run-status';

describe('RunStatus', () => {
  it.each([
    ['SUCCEEDED', 'Ready'],
    ['RUNNING', 'Checking'],
    ['WAITING_FOR_INPUT', 'Needs attention'],
    ['PENDING', 'Not checked'],
    ['FAILED', 'Couldn’t check'],
  ])('renders %s as %s', (status, label) => {
    render(<RunStatus status={status} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });
});
