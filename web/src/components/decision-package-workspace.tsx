import {
  RiCheckLine,
  RiDownloadLine,
  RiErrorWarningLine,
  RiFileCodeLine,
  RiFilePdf2Line,
  RiFileTextLine,
  RiRefreshLine,
} from '@remixicon/react';
import type { ReactNode } from 'react';

import type { Decision, DecisionPackage, EvidenceEntry, ProposedAction } from '@/client/types.gen';
import * as Button from '@/components/ui/button';
import { ActionCenterWorkspace } from '@/components/action-center-workspace';
import { useDecisionPackage } from '@/hooks/use-decision-package';

export function DecisionPackageWorkspace({ decision }: { decision: Decision }) {
  const state = useDecisionPackage(decision.id);

  if (decision.tier === 'UNDETERMINED') return null;

  return (
    <section className="package-workspace" id="decision-package" aria-labelledby="package-title">
      <header className="package-heading">
        <div>
          <span>Decision package</span>
          <h2 id="package-title">Evidence and proposed actions</h2>
          <p>Freeze the current lab, policy, field, placement, and response records into one reviewable package.</p>
        </div>
        {state.value ? (
          <Button.Root variant="neutral" mode="stroke" size="small" onClick={() => void state.generate().catch(() => undefined)} disabled={state.busy !== null}>
            <RiRefreshLine aria-hidden="true" /> {state.busy === 'generate' ? 'Checking…' : 'Generate updated package'}
          </Button.Root>
        ) : null}
      </header>

      {state.error ? <div className="package-error" role="alert"><RiErrorWarningLine aria-hidden="true" /><span>{state.error}</span></div> : null}

      {state.isLoading ? (
        <div className="package-loading"><span className="state-spinner" /><span>Loading the latest package…</span></div>
      ) : state.value ? (
        <>
          <PackageDocument value={state.value} busy={state.busy} onDownload={state.download} />
          <ActionCenterWorkspace packageId={state.value.id} />
        </>
      ) : (
        <div className="package-empty">
          <div><RiFileTextLine aria-hidden="true" /><div><h3>Ready to assemble when the current workflow is complete</h3><p>The package is immutable, source-linked, and clearly separates evidence from proposed actions. It does not approve or execute anything.</p></div></div>
          <Button.Root variant="primary" mode="filled" size="medium" onClick={() => void state.generate().catch(() => undefined)} disabled={state.busy !== null}>
            {state.busy === 'generate' ? 'Generating…' : 'Generate package'}
          </Button.Root>
        </div>
      )}
    </section>
  );
}

function PackageDocument({ value, busy, onDownload }: {
  value: DecisionPackage;
  busy: string | null;
  onDownload: (format: 'html' | 'pdf' | 'json') => Promise<void>;
}) {
  const gaps = value.snapshot.gaps ?? [];
  const ready = value.status === 'READY';
  return (
    <article className="package-document">
      <header className={`package-cover package-cover--${ready ? 'ready' : 'review'}`}>
        <span className="package-cover__icon" aria-hidden="true">{ready ? <RiCheckLine /> : <RiErrorWarningLine />}</span>
        <div><span>Package {shortID(value.id)}</span><h3>{ready ? 'Decision package ready' : 'Decision package needs review'}</h3><p>{ready ? 'The current evidence and proposed actions have been frozen into a replayable record.' : `${gaps.length} open evidence item${gaps.length === 1 ? '' : 's'} remain visible in the package.`}</p></div>
        <dl><div><dt>Sources</dt><dd>{value.evidence?.length ?? 0}</dd></div><div><dt>Actions</dt><dd>{value.proposedActions?.length ?? 0}</dd></div><div><dt>Version</dt><dd>{value.schemaVersion}</dd></div></dl>
      </header>

      <div className="package-exports" aria-label="Download decision package">
        <div><strong>Download the frozen record</strong><span>Each artifact is generated from the same package inputs.</span></div>
        <div>
          <ExportButton icon={<RiFilePdf2Line />} label="PDF" format="pdf" busy={busy} onDownload={onDownload} />
          <ExportButton icon={<RiFileTextLine />} label="HTML" format="html" busy={busy} onDownload={onDownload} />
          <ExportButton icon={<RiFileCodeLine />} label="JSON" format="json" busy={busy} onDownload={onDownload} />
        </div>
      </div>

      {gaps.length ? <section className="package-gaps"><h4>Open evidence</h4>{gaps.map((gap) => <div key={`${gap.source}-${gap.code}`}><span>{gap.source}</span><div><strong>{gap.detail}</strong><p>{gap.resolution}</p></div></div>)}</section> : null}

      <section className="package-section" aria-labelledby="package-actions-title">
        <div className="package-section__heading"><div><span>Proposed actions</span><h4 id="package-actions-title">What happens next</h4></div><small>{value.proposedActions?.length ?? 0} actions</small></div>
        <ol className="package-actions">{value.proposedActions?.map((action) => <ActionRow key={action.code} action={action} />)}</ol>
      </section>

      <details className="package-section package-evidence">
        <summary><div><span>Evidence ledger</span><h4>Sources frozen in this package</h4></div><small>{value.evidence?.length ?? 0} sources</small></summary>
        <ol>{value.evidence?.map((item) => <EvidenceRow key={`${item.position}-${item.kind}`} item={item} />)}</ol>
      </details>

      <footer className="package-boundary">
        <RiCheckLine aria-hidden="true" /><span>Human review only. This package does not approve, submit, schedule, notify, contact, or execute any action.</span>
        <details><summary>Package record</summary><dl><div><dt>Created</dt><dd>{formatDate(value.createdAt)}</dd></div><div><dt>Input hash</dt><dd>{value.inputHash}</dd></div>{value.artifacts?.map((artifact) => <div key={artifact.format}><dt>{artifact.format.toUpperCase()} SHA-256</dt><dd>{artifact.sha256}</dd></div>)}</dl></details>
      </footer>
    </article>
  );
}

function ExportButton({ icon, label, format, busy, onDownload }: {
  icon: ReactNode;
  label: string;
  format: 'html' | 'pdf' | 'json';
  busy: string | null;
  onDownload: (format: 'html' | 'pdf' | 'json') => Promise<void>;
}) {
  return <Button.Root variant="neutral" mode="stroke" size="small" disabled={busy !== null} onClick={() => void onDownload(format).catch(() => undefined)}>{icon}{busy === format ? 'Downloading…' : label}<RiDownloadLine aria-hidden="true" /></Button.Root>;
}

function ActionRow({ action }: { action: ProposedAction }) {
  return <li><span>{String(action.position).padStart(2, '0')}</span><div><strong>{action.title}</strong><p>{action.detail}</p></div><div><b>{action.state === 'ENFORCED' ? 'In effect' : action.state === 'REQUIRED' ? 'Required' : 'Draft'}</b><small>{action.timing}</small></div></li>;
}

function EvidenceRow({ item }: { item: EvidenceEntry }) {
  return <li><span>{String(item.position).padStart(2, '0')}</span><div><strong>{item.title}</strong><p>{item.detail}</p>{item.caveat ? <small>{item.caveat}</small> : null}</div><div><b className={item.status === 'AVAILABLE' ? '' : 'review'}>{item.status === 'AVAILABLE' ? 'Available' : item.status === 'PARTIAL' ? 'Partial' : 'Unavailable'}</b><small>{item.provider}</small>{item.sourceUrl ? <a href={item.sourceUrl} target="_blank" rel="noreferrer">Source</a> : null}</div></li>;
}

function shortID(value: string) { return value.slice(0, 8).toUpperCase(); }
function formatDate(value: string) { return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)); }
