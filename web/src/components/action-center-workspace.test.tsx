import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Center, ControlledAction } from '@/client/types.gen';
import { ActionCenterWorkspace } from '@/components/action-center-workspace';

const mocks = vi.hoisted(() => ({ useActionCenter: vi.fn() }));

vi.mock('@/hooks/use-action-center', () => ({ useActionCenter: mocks.useActionCenter }));

describe('ActionCenterWorkspace', () => {
  beforeEach(() => {
    mocks.useActionCenter.mockReturnValue(state(center([action()])));
  });

  it('keeps deterministic policy controls read-only', () => {
    mocks.useActionCenter.mockReturnValue(state(center([action({
      executionMode: 'CONTROL',
      status: 'EXECUTED',
      approvalRequired: false,
      title: 'Limit the application rate',
    })])));

    render(<ActionCenterWorkspace packageId="package-1" />);

    expect(screen.getByRole('heading', { name: 'Already in effect' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /approve/i })).not.toBeInTheDocument();
  });

  it('requires an approved payload edit to create a new review', () => {
    mocks.useActionCenter.mockReturnValue(state(center([action({
      status: 'APPROVED',
      decisions: [{
        id: 'decision-1',
        kind: 'APPROVED',
        actionRevision: 1,
        payloadHash: 'a'.repeat(64),
        actorName: 'Salar Khan',
        actorRole: 'Prototype operator',
        acknowledgedGapCodes: [],
        createdAt: '2026-08-06T12:00:00Z',
      }],
    })])));

    render(<ActionCenterWorkspace packageId="package-1" />);

    const recipient = screen.getByLabelText('Recipient');
    expect(recipient).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Edit payload' }));
    expect(recipient).toBeEnabled();
    fireEvent.change(recipient, { target: { value: 'Updated operations team' } });
    expect(screen.getByRole('button', { name: 'Save and approve' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled();
  });

  it('shows a completed handoff without claiming an external send', () => {
    mocks.useActionCenter.mockReturnValue(state(center([action({
      status: 'EXECUTED',
      execution: {
        id: 'execution-1',
        outcome: 'OPERATOR_HANDOFF_READY',
        summary: 'The exact approved payload was frozen into an operator handoff. No external party was contacted.',
        externalEffect: false,
        handoffUrl: '/api/v1/execution-attempts/execution-1/handoff',
        completedAt: '2026-08-06T12:05:00Z',
      },
    })])));

    render(<ActionCenterWorkspace packageId="package-1" />);

    expect(screen.getByRole('heading', { name: 'Handoff ready' })).toBeInTheDocument();
    expect(screen.getByText(/No external party was contacted/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Download handoff' })).toBeEnabled();
  });
});

function action(overrides: Partial<ControlledAction> = {}): ControlledAction {
  return {
    id: 'action-1',
    packageId: 'package-1',
    position: 1,
    code: 'VERIFY_COLLECTION_SYSTEM',
    category: 'Source reduction',
    title: 'Verify collection-system connections',
    detail: 'Compare the industrial user inventory and sewer map with the geographic leads.',
    timing: 'Before targeted sampling',
    sourceId: 'response-1',
    executionMode: 'OPERATOR_HANDOFF',
    status: 'PROPOSED',
    approvalRequired: true,
    payload: {
      channel: 'INTERNAL_WORK_ORDER',
      recipient: 'Facility pretreatment and operations team',
      subject: 'Verify collection-system connections',
      message: 'Compare the industrial user inventory and sewer map with the geographic leads.',
      attachments: [],
    },
    revision: 1,
    payloadHash: 'a'.repeat(64),
    decisions: [],
    createdAt: '2026-08-06T12:00:00Z',
    updatedAt: '2026-08-06T12:00:00Z',
    ...overrides,
  };
}

function center(actions: ControlledAction[]): Center {
  return {
    packageId: 'package-1',
    packageHash: 'b'.repeat(64),
    packageStatus: 'READY',
    criticalGaps: [],
    reviewGaps: [],
    actions,
    approvalPolicy: 'Critical gaps block approval. Review gaps require an explicit acknowledgement.',
  };
}

function state(value: Center) {
  return {
    value,
    isLoading: false,
    busy: null,
    error: null,
    save: vi.fn(),
    decide: vi.fn(),
    execute: vi.fn(),
    download: vi.fn(),
  };
}
