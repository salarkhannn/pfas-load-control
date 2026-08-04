import { RiCheckLine, RiErrorWarningLine, RiExternalLinkLine, RiLoader4Line } from '@remixicon/react';

import type { Step, ToolCall } from '@/client/types.gen';
import { RunStatus } from '@/components/run-status';

const checkCopy: Record<string, { label: string; description: string; sourceLabel: string }> = {
  'mireye.meta.fields': {
    label: 'Property data',
    description: 'Available property information and its source details.',
    sourceLabel: 'Mireye property data source',
  },
  'mireye.meta.plans': {
    label: 'Usage pricing',
    description: 'Published credit costs for available account plans.',
    sourceLabel: 'Mireye pricing source',
  },
  'mireye.users.me.usage': {
    label: 'Account access',
    description: 'Current plan, available credits, and renewal date.',
    sourceLabel: 'Mireye account source',
  },
};

const expectedSteps: Step[] = [
  { id: 'expected-fields', position: 1, toolName: 'mireye.meta.fields', status: 'PENDING', attemptCount: 0 },
  { id: 'expected-plans', position: 2, toolName: 'mireye.meta.plans', status: 'PENDING', attemptCount: 0 },
  { id: 'expected-usage', position: 3, toolName: 'mireye.users.me.usage', status: 'PENDING', attemptCount: 0 },
];

export function ReadinessChecks({ steps, calls }: { steps: Step[] | null; calls: ToolCall[] | null }) {
  const visibleSteps = steps ?? expectedSteps;
  const completedChecks = visibleSteps.filter((step) => step.status === 'SUCCEEDED').length;

  return (
    <section className="checks" aria-labelledby="checks-title">
      <div className="checks__header">
        <h2 id="checks-title">Checks</h2>
        <span>{completedChecks} of 3 verified</span>
      </div>

      <ol className="check-list">
        {visibleSteps.map((step) => {
          const copy = checkCopy[step.toolName] ?? {
            label: 'Required data',
            description: 'Availability of information required for an investigation.',
            sourceLabel: 'Mireye source',
          };
          const call = latestCall(calls, step.id);

          return (
            <li className={`check-item check-item--${step.status.toLowerCase()}`} key={step.id}>
              <span className="check-indicator" aria-hidden="true">
                <CheckIcon status={step.status} />
              </span>
              <div className="check-item__body">
                <div className="check-item__title">
                  <h3>{copy.label}</h3>
                  <RunStatus status={step.status} />
                </div>
                <p>{copy.description}</p>
                {step.summary ? <CheckSummary toolName={step.toolName} summary={step.summary} /> : null}
                {call ? <SourceProof call={call} sourceLabel={copy.sourceLabel} /> : null}
              </div>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

function CheckIcon({ status }: { status: string }) {
  if (status === 'SUCCEEDED') return <RiCheckLine />;
  if (status === 'RUNNING') return <RiLoader4Line className="spin" />;
  if (status === 'FAILED' || status === 'WAITING_FOR_INPUT') return <RiErrorWarningLine />;
  return null;
}

function CheckSummary({ toolName, summary }: { toolName: string; summary: unknown }) {
  if (!isRecord(summary)) return null;

  if (toolName === 'mireye.meta.fields') {
    return (
      <dl className="check-metrics">
        <Metric label="Property fields" value={formattedNumber(summary.fieldCount)} />
        <Metric label="Prepared groups" value={formattedNumber(summary.presetCount)} />
      </dl>
    );
  }
  if (toolName === 'mireye.meta.plans') {
    return (
      <dl className="check-metrics">
        <Metric label="Plans" value={formattedNumber(summary.planCount)} />
        <Metric label="Pricing" value={summary.hasCreditCosts === true ? 'Published' : 'Unavailable'} />
      </dl>
    );
  }
  if (toolName === 'mireye.users.me.usage') {
    return (
      <dl className="check-metrics">
        <Metric label="Plan" value={summary.planName} />
        <Metric label="Credits available" value={formattedNumber(summary.creditsRemaining)} />
        <Metric label="Resets" value={formattedDate(summary.resetsAt)} />
      </dl>
    );
  }
  return null;
}

function SourceProof({ call, sourceLabel }: { call: ToolCall; sourceLabel: string }) {
  const checkedLabel = call.status === 'SUCCEEDED' ? 'Checked' : 'Attempted';

  return (
    <div className="source-proof">
      <a href={call.sourceUrl} target="_blank" rel="noreferrer">
        {sourceLabel}
        <RiExternalLinkLine aria-hidden="true" />
      </a>
      <time dateTime={call.fetchedAt}>{checkedLabel} {formatTimestamp(call.fetchedAt)}</time>
      <details className="technical-details">
        <summary>Technical details</summary>
        <dl>
          <EvidenceValue label="Endpoint" value={call.path} code />
          <EvidenceValue label="Request ID" value={call.requestId ?? 'Not returned'} code />
          <EvidenceValue label="Response SHA-256" value={call.responseHash ?? 'Not available'} code />
          <EvidenceValue label="HTTP status" value={call.httpStatus?.toString() ?? 'Not returned'} />
          <EvidenceValue label="Response time" value={`${call.durationMs.toLocaleString()} ms`} />
          <EvidenceValue label="Credits used" value={call.creditCost.toLocaleString()} />
        </dl>
      </details>
    </div>
  );
}

function EvidenceValue({ label, value, code = false }: { label: string; value: string; code?: boolean }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={code ? 'code-value' : undefined}>{value}</dd>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: unknown }) {
  if (typeof value !== 'string' && typeof value !== 'number') return null;
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function latestCall(calls: ToolCall[] | null, stepID: string): ToolCall | undefined {
  return calls
    ?.filter((call) => call.stepId === stepID)
    .sort((left, right) => right.attempt - left.attempt)[0];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function formattedNumber(value: unknown): string {
  return typeof value === 'number' ? value.toLocaleString() : '—';
}

function formattedDate(value: unknown): string {
  if (typeof value !== 'string') return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date);
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'at an unknown time';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date);
}
