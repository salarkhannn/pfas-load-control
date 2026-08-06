import {
	RiCheckLine,
	RiCloseLine,
	RiDownloadLine,
	RiErrorWarningLine,
	RiLockLine,
	RiShieldCheckLine,
} from '@remixicon/react';
import { useState } from 'react';

import type { ControlledAction } from '@/client/types.gen';
import * as Button from '@/components/ui/button';
import { useActionCenter } from '@/hooks/use-action-center';

type Draft = { recipient: string; subject: string; message: string };

export function ActionCenterWorkspace({ packageId }: { packageId: string }) {
	const state = useActionCenter(packageId);
	const actions = state.value?.actions ?? [];
	const [selectedId, setSelectedId] = useState('');

	if (state.isLoading) return <div className="action-center-loading"><span className="state-spinner" /> Loading approvals…</div>;
	if (!state.value) return <div className="action-center-error" role="alert"><RiErrorWarningLine aria-hidden="true" />{state.error ?? 'The approval workspace could not be loaded.'}</div>;

	const selected = actions.find((item) => item.id === selectedId) ?? actions.find((item) => item.status !== 'EXECUTED') ?? actions[0];
	const completed = actions.filter((item) => item.status === 'EXECUTED').length;

	return (
		<section className="action-center" aria-labelledby="action-center-title">
			<header className="action-center__heading">
				<div><span>Controlled execution</span><h2 id="action-center-title">Approve what happens next</h2><p>Review the exact payload. Nothing is released or prepared until you approve it.</p></div>
				<p><strong>{completed}</strong> of {actions.length} complete</p>
			</header>

			{state.error ? <div className="action-center-error" role="alert"><RiErrorWarningLine aria-hidden="true" />{state.error}</div> : null}
			{(state.value.criticalGaps?.length ?? 0) > 0 ? <GapBanner kind="critical" count={state.value.criticalGaps?.length ?? 0} /> : null}

			<div className="action-console">
				<nav className="action-queue" aria-label="Proposed actions">
					<div><strong>Actions</strong><span>{actions.length}</span></div>
					{actions.map((action) => <button key={action.id} type="button" className={action.id === selected?.id ? 'selected' : ''} onClick={() => setSelectedId(action.id)} aria-current={action.id === selected?.id ? 'true' : undefined}><span>{String(action.position).padStart(2, '0')}</span><span><strong>{action.title}</strong><small>{modeLabel(action.executionMode)}</small></span><StateMark status={action.status} /></button>)}
				</nav>
				{selected ? <ActionReview key={`${selected.id}:${selected.revision}:${selected.status}`} action={selected} reviewGapCount={state.value.reviewGaps?.length ?? 0} criticalGapCount={state.value.criticalGaps?.length ?? 0} busy={state.busy} onSave={state.save} onDecide={state.decide} onExecute={state.execute} onDownload={state.download} /> : null}
			</div>
		</section>
	);
}

function ActionReview({ action, reviewGapCount, criticalGapCount, busy, onSave, onDecide, onExecute, onDownload }: {
	action: ControlledAction;
	reviewGapCount: number;
	criticalGapCount: number;
	busy: string | null;
	onSave: (id: string, payload: Draft) => Promise<ControlledAction>;
	onDecide: (kind: 'approve' | 'reject', id: string, input: { expectedPayloadHash: string; actorName: string; actorRole: string; note?: string; acknowledgeGaps: boolean }) => Promise<ControlledAction>;
	onExecute: (id: string) => Promise<ControlledAction>;
	onDownload: (executionId: string) => Promise<void>;
}) {
	const [draft, setDraft] = useState<Draft>({ recipient: action.payload.recipient, subject: action.payload.subject, message: action.payload.message });
	const previousReviewer = action.decisions?.at(-1);
	const [actorName, setActorName] = useState(previousReviewer?.actorName ?? '');
	const [actorRole, setActorRole] = useState(previousReviewer?.actorRole ?? '');
	const [note, setNote] = useState('');
	const [acknowledge, setAcknowledge] = useState(false);
	const [editingApproved, setEditingApproved] = useState(false);
	const dirty = draft.recipient.trim() !== action.payload.recipient || draft.subject.trim() !== action.payload.subject || draft.message.trim() !== action.payload.message;
	const pending = busy?.endsWith(action.id) ?? false;
	const validDraft = draft.subject.trim().length >= 2 && draft.message.trim().length >= 2;
	const completePayload = draft.recipient.trim().length >= 2 && draft.subject.trim().length >= 2 && draft.message.trim().length >= 2;
	const identifiedReviewer = actorName.trim().length >= 2 && actorRole.trim().length >= 2;
	const revisedApproval = action.status !== 'APPROVED' || dirty;
	const canApprove = completePayload && identifiedReviewer && revisedApproval && criticalGapCount === 0 && (reviewGapCount === 0 || acknowledge);
	const canReject = validDraft && identifiedReviewer && note.trim().length >= 2 && revisedApproval;
	const payloadLocked = action.status === 'APPROVED' && !editingApproved;

	async function currentAction() {
		return dirty ? onSave(action.id, { recipient: draft.recipient.trim(), subject: draft.subject.trim(), message: draft.message.trim() }) : action;
	}

	async function decide(kind: 'approve' | 'reject') {
		const current = await currentAction();
		await onDecide(kind, action.id, { expectedPayloadHash: current.payloadHash, actorName: actorName.trim(), actorRole: actorRole.trim(), note: note.trim() || undefined, acknowledgeGaps: acknowledge });
	}

	if (action.executionMode === 'CONTROL') {
		return <article className="action-review action-review--control"><ReviewHeader action={action} /><div className="action-control-state"><RiShieldCheckLine aria-hidden="true" /><div><h3>Already in effect</h3><p>{action.detail}</p><small>This deterministic control was applied by the policy engine and cannot be edited here.</small></div></div><PayloadRecord action={action} /></article>;
	}

	return (
		<article className="action-review">
			<ReviewHeader action={action} />
			{action.execution ? <ExecutionState action={action} pending={pending} onDownload={onDownload} /> : (
				<>
					<div className="action-payload">
						<label>Recipient<input value={draft.recipient} onChange={(event) => setDraft({ ...draft, recipient: event.target.value })} disabled={payloadLocked} placeholder={recipientPlaceholder(action.payload.channel)} /></label>
						<label>Subject<input value={draft.subject} onChange={(event) => setDraft({ ...draft, subject: event.target.value })} disabled={payloadLocked} /></label>
						<label className="wide">Message<textarea rows={6} value={draft.message} onChange={(event) => setDraft({ ...draft, message: event.target.value })} disabled={payloadLocked} /></label>
					</div>
					<div className="action-attachments"><strong>Attached evidence</strong>{action.payload.attachments?.map((item) => <a key={item.sha256} href={item.url} target="_blank" rel="noreferrer"><span>{item.label}</span><small>{item.sha256.slice(0, 12)}…</small></a>)}</div>

					{action.status === 'APPROVED' && !editingApproved ? <ApprovedState action={action} pending={pending} onEdit={() => setEditingApproved(true)} onExecute={onExecute} /> : (
						<div className="action-decision">
							<div className="action-reviewer"><label>Your name<input value={actorName} onChange={(event) => setActorName(event.target.value)} autoComplete="name" /></label><label>Your role<input value={actorRole} onChange={(event) => setActorRole(event.target.value)} /></label><label className="wide">Review note <span>optional for approval</span><textarea rows={2} value={note} onChange={(event) => setNote(event.target.value)} /></label></div>
							{reviewGapCount > 0 ? <label className="action-ack"><input type="checkbox" checked={acknowledge} onChange={(event) => setAcknowledge(event.target.checked)} /><span>I reviewed the {reviewGapCount} open evidence item{reviewGapCount === 1 ? '' : 's'} in the frozen package.</span></label> : null}
							{criticalGapCount > 0 ? <p className="action-blocked"><RiLockLine aria-hidden="true" /> Resolve the critical evidence before approval.</p> : null}
							<div className="action-buttons">
								<Button.Root variant="error" mode="stroke" size="small" disabled={pending || !canReject} onClick={() => void decide('reject').catch(() => undefined)}><RiCloseLine aria-hidden="true" /> Reject</Button.Root>
								{editingApproved ? <Button.Root variant="neutral" mode="stroke" size="small" disabled={pending} onClick={() => { setDraft({ recipient: action.payload.recipient, subject: action.payload.subject, message: action.payload.message }); setEditingApproved(false); }}>Cancel</Button.Root> : null}
								{dirty ? <Button.Root variant="neutral" mode="stroke" size="small" disabled={pending || !validDraft} onClick={() => void currentAction().catch(() => undefined)}>Save changes</Button.Root> : null}
								<Button.Root variant="primary" mode="filled" size="small" disabled={pending || !canApprove} onClick={() => void decide('approve').catch(() => undefined)}><RiCheckLine aria-hidden="true" /> {dirty ? 'Save and approve' : 'Approve exact payload'}</Button.Root>
							</div>
						</div>
					)}
				</>
			)}
			<footer className="action-audit"><span>Revision {action.revision}</span><code>{action.payloadHash}</code></footer>
		</article>
	);
}

function ReviewHeader({ action }: { action: ControlledAction }) {
	return <header className="action-review__header"><div><span>{modeLabel(action.executionMode)}</span><h3>{action.title}</h3><p>{action.detail}</p></div><StateMark status={action.status} label /></header>;
}

function ApprovedState({ action, pending, onEdit, onExecute }: { action: ControlledAction; pending: boolean; onEdit: () => void; onExecute: (id: string) => Promise<ControlledAction> }) {
	const approval = action.decisions?.findLast((item) => item.kind === 'APPROVED' && item.actionRevision === action.revision);
	return <section className="action-approved"><div><RiCheckLine aria-hidden="true" /><div><strong>Approved by {approval?.actorName}</strong><span>{approval?.actorRole} · {approval ? formatDate(approval.createdAt) : ''}</span></div></div><div className="action-approved__buttons"><Button.Root variant="neutral" mode="stroke" size="small" disabled={pending} onClick={onEdit}>Edit payload</Button.Root><Button.Root variant="primary" mode="filled" size="small" disabled={pending} onClick={() => void onExecute(action.id).catch(() => undefined)}>{pending ? 'Completing…' : action.executionMode === 'INTERNAL_RELEASE' ? 'Release plan' : 'Prepare handoff'}</Button.Root></div><p>{action.executionMode === 'INTERNAL_RELEASE' ? 'Releases the frozen plan inside this workspace. It does not schedule field work.' : 'Creates a downloadable operator handoff. It does not contact the recipient.'}</p></section>;
}

function ExecutionState({ action, pending, onDownload }: { action: ControlledAction; pending: boolean; onDownload: (id: string) => Promise<void> }) {
	return <section className="action-executed"><RiCheckLine aria-hidden="true" /><div><h3>{action.execution?.outcome === 'INTERNAL_RELEASED' ? 'Plan released' : 'Handoff ready'}</h3><p>{action.execution?.summary}</p><small>{action.execution ? formatDate(action.execution.completedAt) : ''}</small></div>{action.execution?.handoffUrl ? <Button.Root variant="neutral" mode="stroke" size="small" disabled={pending} onClick={() => void onDownload(action.execution!.id).catch(() => undefined)}><RiDownloadLine aria-hidden="true" /> Download handoff</Button.Root> : null}</section>;
}

function PayloadRecord({ action }: { action: ControlledAction }) {
	return <dl className="action-record"><div><dt>Channel</dt><dd>{channelLabel(action.payload.channel)}</dd></div><div><dt>Recipient</dt><dd>{action.payload.recipient}</dd></div><div><dt>Timing</dt><dd>{action.timing}</dd></div></dl>;
}

function GapBanner({ kind, count }: { kind: 'critical'; count: number }) {
	return <div className={`action-gap action-gap--${kind}`}><RiLockLine aria-hidden="true" /><div><strong>Approval is blocked</strong><span>Resolve {count} critical evidence item{count === 1 ? '' : 's'} in the frozen package first.</span></div></div>;
}

function StateMark({ status, label = false }: { status: string; label?: boolean }) {
	const copy = status === 'EXECUTED' ? 'Complete' : status === 'APPROVED' ? 'Approved' : status === 'REJECTED' ? 'Rejected' : 'Review';
	return <span className={`action-state action-state--${status.toLowerCase()}`} aria-label={copy}>{status === 'EXECUTED' || status === 'APPROVED' ? <RiCheckLine aria-hidden="true" /> : status === 'REJECTED' ? <RiCloseLine aria-hidden="true" /> : <span aria-hidden="true" />}{label ? copy : null}</span>;
}

function modeLabel(mode: string) { return mode === 'INTERNAL_RELEASE' ? 'Internal release' : mode === 'CONTROL' ? 'Policy control' : 'Operator handoff'; }
function channelLabel(channel: string) { return channel.split('_').map((part) => part.charAt(0) + part.slice(1).toLowerCase()).join(' '); }
function recipientPlaceholder(channel: string) { return channel === 'MIENVIRO' ? 'Michigan EGLE' : channel === 'SAMPLING_REQUEST' ? 'Laboratory or sampling team' : 'Exact person, team, or facility'; }
function formatDate(value: string) { return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)); }
