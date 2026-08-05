import {
  RiCheckLine,
  RiErrorWarningLine,
  RiExternalLinkLine,
  RiRefreshLine,
  RiScan2Line,
} from '@remixicon/react';

import type { Evaluation, FieldFact, SupplementalEvidence } from '@/client/types.gen';
import * as Button from '@/components/ui/button';

const CATEGORIES = [
  { key: 'WATER', label: 'Water and wetlands' },
  { key: 'SOIL', label: 'Soil and slope' },
  { key: 'PEOPLE', label: 'Nearby homes and schools' },
  { key: 'LAND', label: 'Land use' },
  { key: 'ACCESS', label: 'Road access' },
] as const;

export function PhysicalEvidence({
  evaluation,
  isLoading,
  isStarting,
  error,
  onStart,
}: {
  evaluation: Evaluation | null;
  isLoading: boolean;
  isStarting: boolean;
  error: string | null;
  onStart: () => Promise<unknown>;
}) {
  if (isLoading) {
    return (
      <section className="physical-evidence physical-evidence--loading" aria-label="Physical evidence">
        <span className="state-spinner" aria-hidden="true" />
        <div><strong>Loading physical evidence</strong><p>Checking for an existing field record.</p></div>
      </section>
    );
  }

  if (!evaluation) {
    return (
      <section className="physical-evidence physical-evidence--empty" aria-labelledby="physical-evidence-title">
        <div className="physical-evidence__intro">
          <span className="physical-evidence__icon"><RiScan2Line aria-hidden="true" /></span>
          <div>
            <h4 id="physical-evidence-title">Check physical conditions</h4>
            <p>Screen the whole field for water, soil, nearby people, land use, and access using cited Mireye data.</p>
          </div>
        </div>
        <Button.Root variant="primary" mode="filled" size="small" disabled={isStarting} onClick={() => void onStart()}>
          {isStarting ? 'Starting…' : 'Check field'}
        </Button.Root>
        {error ? <p className="physical-evidence__error" role="alert">{error}</p> : null}
      </section>
    );
  }

  const active = evaluation.status === 'QUEUED' || evaluation.status === 'RUNNING';
  const review = evaluation.status === 'REVIEW_REQUIRED';
  const failed = evaluation.status === 'FAILED';
  const criticalGaps = evaluation.gaps?.filter((gap) => gap.critical) ?? [];
  const factCount = evaluation.facts?.length ?? 0;

  return (
    <section className="physical-evidence" aria-labelledby="physical-evidence-title" aria-busy={active}>
      <header className={`physical-evidence__status physical-evidence__status--${active ? 'active' : review || failed ? 'review' : 'complete'}`}>
        <span className="physical-evidence__icon">
          {active ? <span className="state-spinner" aria-hidden="true" /> : review || failed ? <RiErrorWarningLine aria-hidden="true" /> : <RiCheckLine aria-hidden="true" />}
        </span>
        <div>
          <p>Physical evidence</p>
          <h4 id="physical-evidence-title">{statusTitle(evaluation.status)}</h4>
          <span>{statusSummary(evaluation, factCount, criticalGaps.length)}</span>
        </div>
        {!active ? (
          <Button.Root variant="neutral" mode="stroke" size="small" disabled={isStarting} onClick={() => void onStart()}>
            <RiRefreshLine aria-hidden="true" /> {isStarting ? 'Starting…' : 'Check again'}
          </Button.Root>
        ) : null}
      </header>

      {error ? <p className="physical-evidence__error" role="alert">{error}</p> : null}
      {evaluation.failureDetail ? <p className="physical-evidence__failure" role="alert">{evaluation.failureDetail}</p> : null}

      {!active && criticalGaps.length > 0 ? (
        <div className="evidence-gaps">
          <strong>{criticalGaps.length} important {criticalGaps.length === 1 ? 'gap needs' : 'gaps need'} review</strong>
          <ul>{criticalGaps.map((gap) => <li key={gap.code}>{gap.detail}</li>)}</ul>
        </div>
      ) : null}

      {!active && factCount > 0 ? (
        <div className="evidence-groups">
          {CATEGORIES.map((category, index) => {
            const facts = evaluation.facts?.filter((fact) => fact.category === category.key) ?? [];
            if (!facts.length) return null;
            const incomplete = facts.filter((fact) => fact.state !== 'COMPLETE').length;
            return (
              <details className="evidence-group" key={category.key} open={index === 0 || incomplete > 0}>
                <summary>
                  <span>{category.label}</span>
                  <small>{incomplete ? `${incomplete} incomplete` : `${facts.length} checked`}</small>
                </summary>
                <div className="evidence-facts">
                  {facts.map((fact) => <EvidenceFact key={fact.name} fact={fact} />)}
                </div>
              </details>
            );
          })}
        </div>
      ) : null}

      {!active && (evaluation.supplemental?.length ?? 0) > 0 ? (
        <section className="supplemental-evidence" aria-labelledby="supplemental-title">
          <div className="supplemental-evidence__heading">
            <h5 id="supplemental-title">Related records</h5>
            <p>Kept separate from Mireye’s physical facts.</p>
          </div>
          {evaluation.supplemental?.map((item) => <SupplementalRow key={`${item.provider}-${item.kind}`} item={item} />)}
        </section>
      ) : null}

      {!active ? (
        <details className="evidence-record">
          <summary>Evidence record</summary>
          <dl>
            <div><dt>Field boundary</dt><dd>Version {evaluation.geometryVersion}</dd></div>
            <div><dt>Field samples</dt><dd>{evaluation.sampleCount}</dd></div>
            <div><dt>Mireye credit ceiling</dt><dd>{evaluation.projectedCredits}</dd></div>
            <div><dt>Completed</dt><dd>{evaluation.completedAt ? formatDate(evaluation.completedAt) : 'Not completed'}</dd></div>
            <div><dt>Field set</dt><dd>{evaluation.fieldSetVersion}</dd></div>
            <div><dt>Aggregation</dt><dd>{evaluation.aggregationVersion}</dd></div>
          </dl>
          {(evaluation.gaps?.length ?? 0) > criticalGaps.length ? (
            <div className="evidence-record__other-gaps">
              <strong>Other source gaps</strong>
              <ul>{evaluation.gaps?.filter((gap) => !gap.critical).map((gap) => <li key={gap.code}>{gap.detail}</li>)}</ul>
            </div>
          ) : null}
        </details>
      ) : null}
    </section>
  );
}

function EvidenceFact({ fact }: { fact: FieldFact }) {
  return (
    <details className={`evidence-fact${fact.state !== 'COMPLETE' ? ' evidence-fact--incomplete' : ''}`}>
      <summary>
        <span>{fact.label}</span>
        <strong>{fact.state === 'UNAVAILABLE' ? 'Unavailable' : formatValue(fact.value, fact.unit)}</strong>
      </summary>
      <div className="evidence-fact__detail">
        <div className="evidence-fact__meta">
          <span>{coverage(fact)}</span>
          {fact.fetchedAt ? <span>Fetched {formatDate(fact.fetchedAt)}</span> : null}
          {fact.sourceUrl ? <a href={fact.sourceUrl} target="_blank" rel="noreferrer">{fact.source || 'Open source'} <RiExternalLinkLine aria-hidden="true" /></a> : fact.source ? <span>{fact.source}</span> : null}
        </div>
        {(fact.samples?.length ?? 0) > 0 ? (
          <div className="sample-evidence" role="table" aria-label={`${fact.label} by field sample`}>
            {fact.samples?.map((sample) => (
              <div role="row" key={`${fact.name}-${sample.index}`}>
                <span role="cell">{sample.label}</span>
                <strong role="cell">{sample.status === 'ok' ? formatValue(sample.value, sample.unit || fact.unit) : sample.status === 'absent' ? 'Not returned' : 'Failed'}</strong>
                <span role="cell">{sample.datasetVintage || sample.notes || sample.error || ''}</span>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </details>
  );
}

function SupplementalRow({ item }: { item: SupplementalEvidence }) {
  return (
    <article className={`supplemental-row${item.status === 'UNAVAILABLE' ? ' supplemental-row--unavailable' : ''}`}>
      <div>
        <span>{item.provider.replaceAll('_', ' ')}</span>
        <h6>{item.title}</h6>
        <p>{item.summary}</p>
        {item.caveat ? <small>{item.caveat}</small> : null}
      </div>
      <a href={item.sourceUrl} target="_blank" rel="noreferrer" aria-label={`Open source for ${item.title}`}>
        Source <RiExternalLinkLine aria-hidden="true" />
      </a>
    </article>
  );
}

function statusTitle(status: string): string {
  if (status === 'QUEUED') return 'Waiting to check the field';
  if (status === 'RUNNING') return 'Checking the whole field';
  if (status === 'SUCCEEDED') return 'Physical evidence ready';
  if (status === 'REVIEW_REQUIRED') return 'Evidence needs review';
  return 'The field check did not complete';
}

function statusSummary(evaluation: Evaluation, factCount: number, criticalGapCount: number): string {
  if (evaluation.status === 'QUEUED') return 'The check will start automatically.';
  if (evaluation.status === 'RUNNING') return `Comparing ${evaluation.sampleCount} points across the confirmed boundary.`;
  if (evaluation.status === 'SUCCEEDED') return `${factCount} conditions checked across ${evaluation.sampleCount} field points.`;
  if (evaluation.status === 'REVIEW_REQUIRED') return criticalGapCount
    ? `${criticalGapCount} important ${criticalGapCount === 1 ? 'gap requires' : 'gaps require'} a human decision.`
    : 'The evidence record needs a human decision.';
  return 'No physical conclusion was produced.';
}

function coverage(fact: FieldFact): string {
  const total = fact.okCount + fact.absentCount + fact.failedCount;
  if (fact.state === 'COMPLETE') return `${fact.okCount} of ${total} field points returned a value`;
  return `${fact.okCount} returned · ${fact.absentCount} absent · ${fact.failedCount} failed`;
}

function formatValue(value: unknown, unit?: string): string {
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  if (typeof value === 'number') return `${formatNumber(value)}${unit ? ` ${formatUnit(unit)}` : ''}`;
  if (typeof value === 'string') return value;
  if (Array.isArray(value)) {
    return value.map((entry) => {
      if (!entry || typeof entry !== 'object') return String(entry);
      const item = entry as { value?: unknown; count?: unknown };
      return `${String(item.value ?? 'Unknown')}${typeof item.count === 'number' && item.count > 1 ? ` (${item.count})` : ''}`;
    }).join(', ');
  }
  if (value && typeof value === 'object') {
    const range = value as { min?: unknown; max?: unknown };
    if (typeof range.min === 'number' && typeof range.max === 'number') {
      const display = range.min === range.max ? formatNumber(range.min) : `${formatNumber(range.min)}–${formatNumber(range.max)}`;
      return `${display}${unit ? ` ${formatUnit(unit)}` : ''}`;
    }
  }
  return 'Not returned';
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(value);
}

function formatUnit(value: string): string {
  return value.replaceAll('_', ' ');
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
