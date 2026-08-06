import { useState, type FormEvent } from 'react';
import {
  RiArrowRightLine,
  RiCheckLine,
  RiErrorWarningLine,
  RiExternalLinkLine,
  RiLoader4Line,
  RiMapPinLine,
} from '@remixicon/react';

import type { AlternativeCandidate, Decision, ResponseRun } from '@/client/types.gen';
import * as Button from '@/components/ui/button';
import { usePfasResponse } from '@/hooks/use-pfas-response';

const activeStatuses = new Set(['QUEUED', 'RUNNING']);

export function ResponseWorkspace({ decision }: { decision: Decision }) {
  const response = usePfasResponse(decision.id);
  const [kind, setKind] = useState<'address' | 'coord'>('address');
  const [input, setInput] = useState('');

  function findLocation(event: FormEvent) {
    event.preventDefault();
    if (input.trim()) void response.resolve(kind, input.trim());
  }

  if (response.isLoading) {
    return <section className="response-workspace response-loading" aria-live="polite"><RiLoader4Line aria-hidden="true" /><span>Loading required response…</span></section>;
  }

  return (
    <section className={`response-workspace response-workspace--${decision.tier.toLowerCase()}`} aria-labelledby="response-title">
      <div className="response-heading">
        <div>
          <span className="response-eyebrow">Required response</span>
          <h2 id="response-title">{decision.tier === 'PROHIBITED' ? 'Control and investigation' : 'Source investigation'}</h2>
          <p>{decision.tier === 'PROHIBITED'
            ? 'Keep this batch out of land application, document the notification path, investigate upstream contributors, and identify facilities to contact about alternative management.'
            : 'Document the required source-reduction work and the evidence needed to investigate upstream contributors.'}</p>
        </div>
      </div>

      {response.error ? <div className="response-error" role="alert"><RiErrorWarningLine aria-hidden="true" /><span>{response.error}</span></div> : null}

      {!response.run ? (
        <div className="response-location">
          <div className="response-location__intro"><RiMapPinLine aria-hidden="true" /><div><h3>Confirm the treatment plant</h3><p>This anchors the wastewater context and upstream investigation to the right facility.</p></div></div>
          {response.location?.disposition === 'resolved' ? (
            <div className="location-match">
              <div>
                <span>Matched location</span>
                <strong>{response.location.resolvedAddress}</strong>
                <small>{countyLabel(response.location.county)}{response.location.confidence ? ` · ${Math.round(response.location.confidence * 100)}% confidence` : ''}</small>
              </div>
              <Button.Root variant="primary" mode="filled" size="medium" disabled={response.busy !== null} onClick={() => void response.confirmAndBuild()}>
                {response.busy === 'start' ? 'Building…' : 'Confirm & build response'} <RiArrowRightLine aria-hidden="true" />
              </Button.Root>
            </div>
          ) : (
            <form className="response-location-form" onSubmit={findLocation}>
              <fieldset><legend>Locate by</legend>
                <label><input type="radio" name="response-location-kind" checked={kind === 'address'} onChange={() => setKind('address')} /><span>Address</span></label>
                <label><input type="radio" name="response-location-kind" checked={kind === 'coord'} onChange={() => setKind('coord')} /><span>Coordinates</span></label>
              </fieldset>
              <label><span>{kind === 'address' ? 'Treatment plant address' : 'Latitude, longitude'}</span><input required maxLength={256} value={input} onChange={(event) => setInput(event.target.value)} placeholder={kind === 'address' ? 'Street, city, Michigan' : '42.1234, -84.5678'} /></label>
              {response.location?.reason ? <p className="location-reason">{response.location.reason}{response.location.hint ? ` ${response.location.hint}` : ''}</p> : null}
              <Button.Root variant="primary" mode="filled" size="medium" type="submit" disabled={!input.trim() || response.busy !== null}>{response.busy === 'resolve' ? 'Finding…' : 'Find facility'}</Button.Root>
            </form>
          )}
        </div>
      ) : <ResponseDossier run={response.run} />}
    </section>
  );
}

function countyLabel(county?: string): string {
  if (!county) return 'Michigan';
  return /\bcounty$/i.test(county.trim()) ? county.trim() : `${county.trim()} County`;
}

function ResponseDossier({ run }: { run: ResponseRun }) {
  if (activeStatuses.has(run.status)) {
    return <div className="response-building" aria-live="polite"><RiLoader4Line aria-hidden="true" /><div><h3>Building the response</h3><p>Checking wastewater context, upstream investigation leads{run.tier === 'PROHIBITED' ? ', and alternative management routes' : ''}.</p></div></div>;
  }
  if (run.status === 'FAILED') {
    return <div className="response-failed" role="alert"><RiErrorWarningLine aria-hidden="true" /><div><h3>Response could not be completed</h3><p>{run.failureDetail ?? 'Retry after the data source is available.'}</p></div></div>;
  }

  const needsReview = run.status === 'REVIEW_REQUIRED';
  return (
    <div className="response-dossier">
      <header className={`response-summary${needsReview ? ' response-summary--review' : ''}`}>
        <span className="response-summary__icon" aria-hidden="true">{needsReview ? '!' : <RiCheckLine />}</span>
        <div><h3>{needsReview ? 'Response needs evidence' : run.tier === 'PROHIBITED' ? 'Land application blocked; response ready' : 'Source-reduction response ready'}</h3><p>{needsReview ? 'The required controls remain in place. Resolve the gaps below before relying on the investigation or management shortlist.' : 'Use this dossier to coordinate the required work. It does not submit notifications or contact facilities.'}</p></div>
      </header>

      {run.dataGaps?.length ? <section className="response-gaps" aria-labelledby="response-gaps-title"><h4 id="response-gaps-title">Resolve before relying on this response</h4>{run.dataGaps.map((gap) => <div key={gap.code}><strong>{gap.detail}</strong><span>{gap.resolution}</span></div>)}</section> : null}

      <section className="response-section" aria-labelledby="response-actions-title">
        <div className="response-section__heading"><div><span>Action ledger</span><h3 id="response-actions-title">What happens next</h3></div><small>{run.tasks?.length ?? 0} actions</small></div>
        <ol className="response-actions">
          {run.tasks?.map((task) => <li key={task.code}><span>{String(task.position).padStart(2, '0')}</span><div><strong>{task.title}</strong><p>{task.detail}</p></div><div><b>{task.state === 'ENFORCED' ? 'In effect' : task.state === 'REQUIRED' ? 'Required' : 'Prepare'}</b><small>{task.timing}</small></div></li>)}
        </ol>
      </section>

      <section className="response-section" aria-labelledby="investigation-title">
        <div className="response-section__heading"><div><span>Upstream investigation</span><h3 id="investigation-title">Potential records to verify</h3></div><small>{run.investigationLeads?.length ?? 0} leads</small></div>
        <p className="response-caveat">These are geographic and industry-sector leads. They do not prove a sewer connection, PFAS use, release, or causation.</p>
        {run.investigationLeads?.length ? <ol className="investigation-list">{run.investigationLeads.map((lead) => <li key={lead.registryId}><span>{String(lead.position).padStart(2, '0')}</span><div><strong>{lead.facilityName}</strong><p>{lead.rationale}</p><small>{lead.city}{lead.city && lead.state ? ', ' : ''}{lead.state}{lead.naicsCodes?.length ? ` · NAICS ${lead.naicsCodes.join(', ')}` : ''}</small></div><a href={lead.sourceUrl} target="_blank" rel="noreferrer" aria-label={`Open EPA record for ${lead.facilityName}`}><RiExternalLinkLine aria-hidden="true" /></a></li>)}</ol> : <p className="response-empty">No geographic sector leads were returned. Review the utility’s current industrial user inventory directly.</p>}
      </section>

      {run.tier === 'PROHIBITED' ? <AlternativeManagement alternatives={run.alternatives ?? []} /> : null}

      <details className="response-evidence">
        <summary>Evidence record</summary>
        <div>{run.evidence?.map((item) => <article key={`${item.provider}-${item.kind}`}><div><span>{item.provider}</span><strong>{item.title}</strong><p>{item.summary}</p><small>{item.caveat}</small></div><a href={item.sourceUrl} target="_blank" rel="noreferrer">Source <RiExternalLinkLine aria-hidden="true" /></a></article>)}</div>
      </details>
    </div>
  );
}

function AlternativeManagement({ alternatives }: { alternatives: AlternativeCandidate[] }) {
  return <section className="response-section" aria-labelledby="alternatives-title">
    <div className="response-section__heading"><div><span>Alternative management</span><h3 id="alternatives-title">Facilities to contact</h3></div><small>{alternatives.length} candidates</small></div>
    <p className="response-caveat">No facility has been contacted. Acceptance, analytical requirements, capacity, price, and scheduling must be confirmed before transport.</p>
    {alternatives.length ? <ol className="alternative-list">{alternatives.map((candidate) => <li key={candidate.wdsId}><span>{String(candidate.position).padStart(2, '0')}</span><div><strong>{candidate.facilityName}</strong><p>{candidate.address}, {candidate.city} · {candidate.disposalAreaStatus}</p><small>{routeText(candidate)}</small></div><span className="acceptance-state">Acceptance unverified</span><a href={candidate.sourceUrl} target="_blank" rel="noreferrer" aria-label={`Open EGLE record for ${candidate.facilityName}`}><RiExternalLinkLine aria-hidden="true" /></a></li>)}</ol> : <p className="response-empty">No candidate facility could be shortlisted from the current EGLE inventory.</p>}
  </section>;
}

function routeText(candidate: AlternativeCandidate): string {
  if (candidate.routeStatus === 'ROUTED' && candidate.drivingDistanceKm !== undefined && candidate.durationMinutes !== undefined) {
    return `${candidate.drivingDistanceKm.toFixed(1)} km · about ${Math.round(candidate.durationMinutes)} minutes by road`;
  }
  if (candidate.routeNote) return candidate.routeNote;
  return `${candidate.straightlineDistanceKm.toFixed(1)} km straight-line prescreen`;
}
