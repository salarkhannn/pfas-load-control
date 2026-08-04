import * as StatusBadge from '@/components/ui/status-badge';

type BadgeStatus = 'completed' | 'pending' | 'failed' | 'disabled';

const labels: Record<string, string> = {
  CANCELLED: 'Cancelled',
  FAILED: 'Couldn’t check',
  PENDING: 'Not checked',
  QUEUED: 'Waiting',
  RUNNING: 'Checking',
  SUCCEEDED: 'Ready',
  WAITING_FOR_INPUT: 'Needs attention',
};

export function RunStatus({ status }: { status: string }) {
  return (
    <StatusBadge.Root variant="stroke" status={badgeStatus(status)}>
      <StatusBadge.Dot />
      {labels[status] ?? 'Status unavailable'}
    </StatusBadge.Root>
  );
}

function badgeStatus(status: string): BadgeStatus {
  if (status === 'SUCCEEDED') return 'completed';
  if (status === 'RUNNING' || status === 'QUEUED') return 'pending';
  if (status === 'FAILED' || status === 'WAITING_FOR_INPUT' || status === 'CANCELLED') return 'failed';
  return 'disabled';
}
