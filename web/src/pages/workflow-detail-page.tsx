import { useCallback, useEffect, useState } from 'react';
import {
  RiArrowLeftLine,
  RiCheckLine,
  RiCloseLine,
  RiErrorWarningLine,
  RiFileUploadLine,
  RiLoader4Line,
  RiUserLine,
} from '@remixicon/react';

import {
  type CoordWorkflow,
  type CoordStep,
  type CoordDocument,
  type Party,
  getWorkflow,
  listWorkflowDocuments,
  assignStep,
  confirmStep,
  rejectStep,
  listParties,
} from '@/api';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { TopNav } from '@/components/top-nav';
import { getWorkspaceKey } from '@/utils/workspace-key';

const STEP_ROLE_LABEL: Record<string, string> = {
  FARMER: 'Farmer confirmation',
  CONTRACTOR: 'Contractor confirmation',
  PLANT: 'Plant confirmation',
};

const STEP_STATUS_LABEL: Record<string, string> = {
  PENDING: 'Waiting',
  CONFIRMED: 'Confirmed',
  REJECTED: 'Rejected',
};

const STATUS_COLOR: Record<string, string> = {
  PENDING: 'var(--fg-subtle)',
  CONFIRMED: 'var(--state-success)',
  REJECTED: 'var(--state-danger)',
};

export function WorkflowDetailPage() {
  const ws = getWorkspaceKey();
  const workflowId = window.location.pathname.split('/').pop()!;
  const [workflow, setWorkflow] = useState<CoordWorkflow | null>(null);
  const [steps, setSteps] = useState<CoordStep[]>([]);
  const [docs, setDocs] = useState<CoordDocument[]>([]);
  const [parties, setParties] = useState<Party[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [acting, setActing] = useState<string | null>(null);

  const loadData = useCallback(async (ws: string, workflowId: string) => {
    const [wfData, docData, partyData] = await Promise.all([
      getWorkflow(ws, workflowId),
      listWorkflowDocuments(ws, workflowId),
      listParties(ws),
    ]);
    return { wfData, docData, partyData };
  }, []);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const data = await loadData(ws, workflowId);
        if (!active) return;
        setWorkflow(data.wfData.workflow);
        setSteps(data.wfData.steps);
        setDocs(data.docData);
        setParties(data.partyData);
        setError(null);
      } catch (e) {
        if (!active) return;
        setError(e instanceof Error ? e.message : 'Failed to load workflow.');
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [ws, workflowId, loadData]);

  const reload = useCallback(async () => {
    try {
      const data = await loadData(ws, workflowId);
      setWorkflow(data.wfData.workflow);
      setSteps(data.wfData.steps);
      setDocs(data.docData);
      setParties(data.partyData);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load workflow.');
    } finally {
      setLoading(false);
    }
  }, [ws, workflowId, loadData]);

  const handleConfirm = async (step: CoordStep) => {
    if (!step.partyId) return;
    setActing(step.id);
    try {
      const updated = await confirmStep(ws, step.id, { partyId: step.partyId, notes: '' });
      setSteps((prev) => prev.map((s) => s.id === updated.id ? updated : s));
      void reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to confirm step.');
    } finally {
      setActing(null);
    }
  };

  const handleReject = async (step: CoordStep, notes: string) => {
    if (!step.partyId) return;
    setActing(step.id);
    try {
      const updated = await rejectStep(ws, step.id, { partyId: step.partyId, notes });
      setSteps((prev) => prev.map((s) => s.id === updated.id ? updated : s));
      void reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to reject step.');
    } finally {
      setActing(null);
    }
  };

  const handleAssigned = useCallback((updated: CoordStep) => {
    setSteps((prev) => prev.map((s) => s.id === updated.id ? updated : s));
    void reload();
  }, [reload]);

  if (loading) {
    return (
      <div className="app-shell">
        <TopNav />
        <main className="workspace coord-workspace">
          <a className="back-button coord-back" href="/coordination"><RiArrowLeftLine aria-hidden="true" /> Workflows</a>
          <div className="coord-loading"><RiLoader4Line className="spin" aria-hidden="true" /> Loading workflow...</div>
        </main>
      </div>
    );
  }

  if (error || !workflow) {
    return (
      <div className="app-shell">
        <TopNav />
        <main className="workspace coord-workspace">
          <a className="back-button coord-back" href="/coordination"><RiArrowLeftLine aria-hidden="true" /> Workflows</a>
          <Alert.Root variant="lighter" status="error" size="large" role="alert">
            <Alert.Icon as={RiErrorWarningLine} />
            <div><strong>Could not load workflow</strong><p>{error || 'Workflow not found.'}</p></div>
          </Alert.Root>
        </main>
      </div>
    );
  }

  const farmerStep = steps.find((s) => s.stepRole === 'FARMER');
  const contractorStep = steps.find((s) => s.stepRole === 'CONTRACTOR');
  const plantStep = steps.find((s) => s.stepRole === 'PLANT');
  const statusColor = STATUS_COLOR[workflow.status] || 'var(--fg-subtle)';

  return (
    <div className="app-shell">
      <TopNav />

      <main className="workspace coord-workspace">
        <a className="back-button coord-back" href="/coordination"><RiArrowLeftLine aria-hidden="true" /> Workflows</a>
        <section className="coord-detail" aria-labelledby="wf-detail-title">
          <div className="page-header">
            <div>
              <p className="eyebrow">Workflow</p>
              <h1 id="wf-detail-title">{workflow.fieldName || 'Unnamed field'}</h1>
              <p className="coord-detail__meta">
                Status: <strong style={{ color: statusColor }}>{workflowStatusLabel(workflow.status)}</strong>
                <span className="coord-detail__sep">|</span>
                Created {formatDate(workflow.createdAt)}
              </p>
            </div>
          </div>

          <Alert.Root className="coord-scope-note" variant="lighter" status="feature" size="large">
            <div><strong>Coordination record</strong><p>These confirmations document coordination only. They do not establish agronomic suitability or authorize biosolids application. Actor authentication is not enabled in this prototype.</p></div>
          </Alert.Root>

          {error && (
            <Alert.Root variant="lighter" status="error" size="large" role="alert">
              <Alert.Icon as={RiErrorWarningLine} />
              <div><strong>Error</strong><p>{error}</p></div>
            </Alert.Root>
          )}

          {/* Step pipeline */}
          <div className="coord-pipeline">
            <StepCard
              step={farmerStep}
              role="FARMER"
              available
              acting={acting}
              parties={parties.filter((p) => p.role === 'FARMER')}
              onConfirm={handleConfirm}
              onReject={handleReject}
              onAssigned={handleAssigned}
            />
            <StepConnector active={farmerStep?.status === 'CONFIRMED'} />
            <StepCard
              step={contractorStep}
              role="CONTRACTOR"
              available={farmerStep?.status === 'CONFIRMED'}
              acting={acting}
              parties={parties.filter((p) => p.role === 'CONTRACTOR')}
              onConfirm={handleConfirm}
              onReject={handleReject}
              onAssigned={handleAssigned}
            />
            <StepConnector active={contractorStep?.status === 'CONFIRMED'} />
            <StepCard
              step={plantStep}
              role="PLANT"
              available={contractorStep?.status === 'CONFIRMED'}
              acting={acting}
              parties={parties.filter((p) => p.role === 'PLANT')}
              onConfirm={handleConfirm}
              onReject={handleReject}
              onAssigned={handleAssigned}
            />
          </div>

          {/* Documents */}
          <section className="coord-docs" aria-labelledby="docs-title">
            <h2 id="docs-title">Documents</h2>
            {docs.length === 0 ? (
              <p className="coord-empty-small">No documents uploaded yet.</p>
            ) : (
              <div className="coord-doc-list">
                {docs.map((d) => (
                  <div key={d.id} className="coord-doc-item">
                    <RiFileUploadLine aria-hidden="true" />
                    <div>
                      <span className="coord-doc__name">{d.filename}</span>
                      <span className="coord-doc__meta">{d.partyName} &middot; {formatDate(d.createdAt)}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        </section>
      </main>
    </div>
  );
}

// ─── Step card ────────────────────────────────────────────────────────

function StepCard({ step, role, acting, available, parties, onConfirm, onReject, onAssigned }: {
  step: CoordStep | undefined;
  role: string;
  acting: string | null;
  available: boolean;
  parties: Party[];
  onConfirm: (step: CoordStep) => void;
  onReject: (step: CoordStep, notes: string) => void;
  onAssigned: (step: CoordStep) => void;
}) {
  const [showAssign, setShowAssign] = useState(false);
  const [selectedPartyId, setSelectedPartyId] = useState(step?.partyId || '');
  const [note, setNote] = useState('');
  const [attested, setAttested] = useState(false);
  const [showReject, setShowReject] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleAssign = async () => {
    if (!selectedPartyId) return;
    setBusy(true);
    try {
      const ws = getWorkspaceKey();
      const updated = await assignStep(ws, step!.id, selectedPartyId);
      onAssigned(updated);
      setShowAssign(false);
      setActionError(null);
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Could not assign this party.');
    } finally {
      setBusy(false);
    }
  };

  const status = step?.status || 'PENDING';
  const statusColor = STATUS_COLOR[status] || 'var(--fg-subtle)';
  const isComplete = status === 'CONFIRMED';
  const isRejected = status === 'REJECTED';
  const canAct = available && !isComplete && !isRejected && !acting && Boolean(step?.partyId);

  return (
    <div className={`coord-step-card${isComplete ? ' coord-step-card--done' : ''}${isRejected ? ' coord-step-card--rejected' : ''}`}>
      <div className="coord-step-card__header">
        <span className="coord-step-card__icon" aria-hidden="true">
          {isComplete ? <RiCheckLine /> : isRejected ? <RiCloseLine /> : <RiUserLine />}
        </span>
        <span className="coord-step-card__role">{STEP_ROLE_LABEL[role] || role}</span>
      </div>

      {step?.partyId ? (
        <div className="coord-step-card__assigned">
          <span className="coord-step-card__party">{step.partyName || step.partyEmail || 'Assigned'}</span>
          <span className="coord-step-card__status" style={{ color: statusColor }}>
            {STEP_STATUS_LABEL[status] || status}
          </span>
        </div>
      ) : (
        <div className="coord-step-card__unassigned">
          <span>No party assigned</span>
          <button className="coord-step-card__link" type="button" onClick={() => setShowAssign(true)}>Assign party</button>
        </div>
      )}

      {showAssign && !step?.partyId && (
        <div className="coord-step-card__form">
          <label htmlFor={`party-${step?.id}`}>Responsible party</label>
          <select id={`party-${step?.id}`} value={selectedPartyId} onChange={(e) => setSelectedPartyId(e.target.value)}>
            <option value="">Select party...</option>
            {parties.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
          <Button.Root variant="primary" mode="filled" size="xxsmall" onClick={handleAssign} disabled={busy || !selectedPartyId}>
            {busy ? 'Assigning...' : 'Assign party'}
          </Button.Root>
        </div>
      )}

      {canAct && (
        <div className="coord-step-card__decision">
          <label className="coord-attestation">
            <input type="checkbox" checked={attested} onChange={(event) => setAttested(event.target.checked)} />
            <span>I reviewed the field facts and confirm this coordination step only. This does not establish agronomic suitability or authorize application.</span>
          </label>
          <div className="coord-step-card__actions">
          <Button.Root variant="primary" mode="filled" size="xxsmall" onClick={() => onConfirm(step!)} disabled={!!acting || !attested}>
            <RiCheckLine aria-hidden="true" /> Confirm
          </Button.Root>
          <Button.Root variant="error" mode="stroke" size="xxsmall" onClick={() => setShowReject(true)} disabled={!!acting}>
            <RiCloseLine aria-hidden="true" /> Reject
          </Button.Root>
          </div>
        </div>
      )}

      {!available && !isComplete && !isRejected && <p className="coord-step-card__waiting">Available after the previous role confirms.</p>}

      {showReject && canAct && (
        <div className="coord-step-card__reject">
          <label htmlFor={`reject-${step!.id}`}>Reason for rejection</label>
          <textarea id={`reject-${step!.id}`} value={note} onChange={(event) => setNote(event.target.value)} rows={3} required />
          <div className="coord-step-card__actions">
            <Button.Root variant="error" mode="filled" size="xxsmall" onClick={() => onReject(step!, note)} disabled={!note.trim() || !!acting}>Record rejection</Button.Root>
            <Button.Root variant="neutral" mode="ghost" size="xxsmall" onClick={() => setShowReject(false)}>Cancel</Button.Root>
          </div>
        </div>
      )}

      {actionError && <p className="coord-step-card__error" role="alert">{actionError}</p>}

      {step?.notes && <p className="coord-step-card__notes">{step.notes}</p>}
      {step?.confirmedAt && <p className="coord-step-card__time">Confirmed {formatDate(step.confirmedAt)}</p>}
    </div>
  );
}

function StepConnector({ active }: { active: boolean }) {
  return <div className={`coord-connector${active ? ' coord-connector--active' : ''}`} aria-hidden="true" />;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  } catch {
    return iso;
  }
}

function workflowStatusLabel(status: string): string {
  if (status === 'PLANT_CONFIRMED' || status === 'READY') return 'Coordination complete';
  return status.replace(/_/g, ' ').toLowerCase().replace(/^./, (letter) => letter.toUpperCase());
}
