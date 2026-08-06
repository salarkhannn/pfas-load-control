import { useState } from 'react';
import {
  RiCheckLine,
  RiErrorWarningLine,
  RiExternalLinkLine,
  RiRefreshLine,
} from '@remixicon/react';

import type { Decision, PlacementComponent, PlacementField, PlacementPlan } from '@/client/types.gen';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { usePlacementPlan } from '@/hooks/use-placement-plan';

const CATEGORY_KEYS = [
  { key: 'WATER_RECEPTORS', label: 'Water' },
  { key: 'SUBSURFACE_MOBILITY', label: 'Ground' },
  { key: 'SURFACE_TRANSPORT', label: 'Runoff' },
  { key: 'HUMAN_FOOD_EXPOSURE', label: 'Exposure' },
  { key: 'DATA_UNCERTAINTY', label: 'Evidence' },
] as const;

export function PlacementWorkspace({ decision }: { decision: Decision }) {
  const state = usePlacementPlan(decision.id);
  const [wetMassKg, setWetMassKg] = useState(decision.wetMassKg ?? '');
  const [percentSolids, setPercentSolids] = useState(decision.percentSolids ?? '');
  const quantityMissing = !decision.wetMassKg || !decision.percentSolids;

  return (
    <section className="placement-workspace" aria-labelledby="placement-title">
      <header className="placement-heading">
        <div>
          <h2 id="placement-title">Draft placement plan</h2>
          <p>Compare confirmed fields and fit this batch within their remaining capacity.</p>
        </div>
        {state.plan ? (
          <Button.Root variant="neutral" mode="stroke" size="small" disabled={state.isBuilding} onClick={() => void state.build()}>
            <RiRefreshLine aria-hidden="true" /> {state.isBuilding ? 'Comparing…' : 'Compare again'}
          </Button.Root>
        ) : null}
      </header>

      {state.error ? (
        <Alert.Root className="error-alert placement-error" variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Couldn’t compare these fields</strong><p>{state.error}</p></div>
        </Alert.Root>
      ) : null}

      {state.isLoading ? <div className="placement-loading"><span className="state-spinner" /><span>Loading the latest plan…</span></div> : state.plan ? (
        <PlacementResult plan={state.plan} />
      ) : (
        <form className="placement-start" onSubmit={(event) => {
          event.preventDefault();
          void state.build(quantityMissing ? { wetMassKg: wetMassKg.trim(), percentSolids: percentSolids.trim() } : {});
        }}>
          <div>
            <strong>{quantityMissing ? 'Add the batch quantity' : 'Ready to compare fields'}</strong>
            <p>{quantityMissing ? 'Wet mass and total solids are used to calculate the dry tons that need a field.' : `${formatNumber(decision.wetMassKg)} kg at ${formatNumber(decision.percentSolids)}% total solids.`}</p>
          </div>
          {quantityMissing ? (
            <div className="placement-quantity">
              <label><span>Wet mass <small>kg</small></span><input required inputMode="decimal" value={wetMassKg} onChange={(event) => setWetMassKg(event.target.value)} /></label>
              <label><span>Total solids <small>%</small></span><input required inputMode="decimal" value={percentSolids} onChange={(event) => setPercentSolids(event.target.value)} /></label>
            </div>
          ) : null}
          <Button.Root type="submit" variant="primary" mode="filled" size="small" disabled={state.isBuilding || (quantityMissing && (!wetMassKg.trim() || !percentSolids.trim()))}>
            {state.isBuilding ? 'Comparing…' : 'Build draft plan'}
          </Button.Root>
        </form>
      )}
    </section>
  );
}

function PlacementResult({ plan }: { plan: PlacementPlan }) {
  const copy = statusCopy(plan);
  const fields = plan.fields ?? [];
  return (
    <div className="placement-result">
      <header className={`placement-summary placement-summary--${copy.tone}`}>
        <span className="placement-summary__symbol" aria-hidden="true">{copy.symbol}</span>
        <div><h3>{copy.title}</h3><p>{copy.detail}</p></div>
        <dl>
          <div><dt>Batch</dt><dd>{formatTons(plan.batchDryTons)}</dd></div>
          <div><dt>Placed</dt><dd>{formatTons(plan.allocatedDryTons)}</dd></div>
          <div><dt>Remaining</dt><dd>{formatTons(plan.unallocatedDryTons)}</dd></div>
        </dl>
      </header>

      {plan.gaps?.length ? (
        <div className="placement-gaps">
          {plan.gaps.map((gap) => <div key={gap.code}><strong>{gap.detail}</strong><span>{gap.resolution}</span></div>)}
        </div>
      ) : null}

      {fields.length ? (
        <div className="placement-ledger" role="table" aria-label="Candidate field comparison">
          <div className="placement-ledger__head" role="row">
            <span role="columnheader">Field</span>
            <span role="columnheader">Water</span>
            <span role="columnheader">Ground</span>
            <span role="columnheader">Runoff</span>
            <span role="columnheader">Exposure</span>
            <span role="columnheader">Evidence</span>
            <span role="columnheader">Capacity</span>
            <span role="columnheader">Draft</span>
          </div>
          {fields.map((field) => <PlacementFieldRow key={field.fieldId} field={field} plan={plan} />)}
        </div>
      ) : <p className="placement-empty">Add a candidate field to compare its capacity and physical conditions.</p>}

      {plan.allocations?.length ? (
        <section className="allocation-list" aria-labelledby="allocation-title">
          <div className="allocation-list__heading"><h3 id="allocation-title">Proposed allocation</h3><span>{plan.allocations.length} field{plan.allocations.length === 1 ? '' : 's'}</span></div>
          {plan.allocations.map((allocation) => (
            <div className="allocation-row" key={allocation.fieldId}>
              <span>{String(allocation.position).padStart(2, '0')}</span>
              <strong>{allocation.fieldName}</strong>
              <p>{formatNumber(allocation.acres)} acres at {formatNumber(allocation.rateDryTonsPerAcre)} dry tons/acre</p>
              <b>{formatTons(allocation.dryTons)}</b>
            </div>
          ))}
        </section>
      ) : null}

      <footer className="placement-boundary">
        <RiCheckLine aria-hidden="true" /> Draft only. Review against the approved RMP before scheduling any application.
        <details><summary>Decision record</summary><dl><div><dt>Plan</dt><dd>{plan.id}</dd></div><div><dt>Inputs</dt><dd>{plan.inputHash}</dd></div><div><dt>Method</dt><dd>{plan.configVersion}</dd></div><div><dt>Method checksum</dt><dd>{plan.configChecksum}</dd></div></dl></details>
      </footer>
    </div>
  );
}

function PlacementFieldRow({ field, plan }: { field: PlacementField; plan: PlacementPlan }) {
  const allocation = plan.allocations?.find((item) => item.fieldId === field.fieldId);
  const categories = new Map(field.categories?.map((category) => [category.key, category]) ?? []);
  return (
    <details className="placement-field-row" role="row">
      <summary>
        <span className="placement-field-name"><i>{field.rank ? String(field.rank).padStart(2, '0') : '—'}</i><span><strong>{field.fieldName}</strong><small>{dispositionLabel(field.disposition)}</small></span></span>
        {CATEGORY_KEYS.map((category) => <Band key={category.key} label={category.label} value={categories.get(category.key)?.band ?? 'UNKNOWN'} />)}
        <span className="placement-number" data-label="Capacity">{formatTons(field.availableCapacityDryTons)}</span>
        <span className="placement-number placement-number--strong" data-label="Draft">{formatTons(allocation?.dryTons)}</span>
      </summary>
      <div className="placement-field-detail">
        <div className="placement-field-reason"><strong>{field.explanation}</strong>{field.counterfactual ? <p>{field.counterfactual}</p> : null}</div>
        <div className="vulnerability-list">
          {field.categories?.map((category) => (
            <div key={category.key}>
              <span><Band value={category.band} /><strong>{category.label}</strong></span>
              <p>{category.explanation}</p>
              <EvidenceLine components={category.components ?? []} />
              {category.sourceUrl ? <a href={category.sourceUrl} target="_blank" rel="noreferrer">{category.sourceTitle} <RiExternalLinkLine aria-hidden="true" /></a> : <small>{category.sourceTitle}</small>}
            </div>
          ))}
        </div>
      </div>
    </details>
  );
}

function EvidenceLine({ components }: { components: PlacementComponent[] }) {
  const visible = components.filter((item) => item.state === 'COMPLETE').slice(0, 4);
  if (!visible.length) return <small>No complete supporting value was returned.</small>;
  return <small>{visible.map((item) => `${item.label}: ${formatValue(item.value, item.unit)}`).join(' · ')}</small>;
}

function Band({ value, label }: { value: string; label?: string }) {
  return <span className={`placement-band placement-band--${value.toLowerCase()}`} data-band={value} data-label={label}>{bandLabel(value)}</span>;
}

function statusCopy(plan: PlacementPlan) {
  const eligibleFields = plan.fields?.filter((field) => field.disposition === 'ELIGIBLE').length ?? 0;
  switch (plan.status) {
    case 'READY': return { title: 'The batch fits', detail: 'All dry tons fit within confirmed eligible field capacity.', symbol: '✓', tone: 'ready' };
    case 'REVIEW_REQUIRED': return { title: 'Field evidence needs review', detail: 'Resolve the listed field issue before assigning the remaining batch.', symbol: '?', tone: 'warning' };
    case 'INSUFFICIENT_CAPACITY': return eligibleFields === 0
      ? { title: 'No eligible field is available', detail: 'Add an approved field before assigning this batch.', symbol: '!', tone: 'warning' }
      : { title: 'More field capacity is needed', detail: 'The eligible fields cannot receive the entire batch without exceeding their confirmed limits.', symbol: '!', tone: 'warning' };
    case 'LAND_APPLICATION_BLOCKED': return { title: 'Land application is blocked', detail: 'No dry tons were assigned to fields.', symbol: '×', tone: 'danger' };
    default: return { title: 'The plan needs review', detail: 'Resolve the listed issue before relying on an allocation.', symbol: '?', tone: 'warning' };
  }
}

function dispositionLabel(value: string) {
  if (value === 'ELIGIBLE') return 'Eligible';
  if (value === 'INELIGIBLE') return 'Not eligible';
  return 'Needs review';
}

function bandLabel(value: string) {
  if (value === 'MODERATE') return 'Medium';
  if (value === 'UNKNOWN') return 'Unknown';
  return value.charAt(0) + value.slice(1).toLowerCase();
}

function formatTons(value?: string) {
  return value ? `${formatNumber(value)} dry tons` : '—';
}

function formatNumber(value?: string) {
  if (!value) return '—';
  const number = Number(value);
  return Number.isFinite(number) ? new Intl.NumberFormat('en-US', { maximumFractionDigits: 3 }).format(number) : value;
}

function formatValue(value: unknown, unit?: string) {
  if (value === undefined || value === null) return 'not returned';
  if (typeof value === 'object') return JSON.stringify(value);
  return `${String(value)}${unit ? ` ${unit}` : ''}`;
}
