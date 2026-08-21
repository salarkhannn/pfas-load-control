import { useEffect, useId, useMemo, useState } from 'react';
import { RiArrowLeftLine, RiCheckLine, RiErrorWarningLine, RiFileTextLine, RiShieldCheckLine } from '@remixicon/react';

import { loadLabReportFile } from '@/api';
import type { Analyte, CorrectionWritable, Report } from '@/client/types.gen';
import { PolicyDecisionView } from '@/components/policy-decision';
import { TopNav } from '@/components/top-nav';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { useLabReport } from '@/hooks/use-lab-report';

export function LabEvidencePage() {
  const state = useLabReport();
  const [showEvidence, setShowEvidence] = useState(false);

  return (
    <div className="app-shell lab-shell">
      <TopNav />

      <main className="workspace page-content lab-workspace">
        {state.error ? (
          <Alert.Root className="error-alert lab-error" variant="lighter" status="error" size="large" role="alert">
            <Alert.Icon as={RiErrorWarningLine} />
            <div><strong>Something needs attention</strong><p>{state.error}</p></div>
          </Alert.Root>
        ) : null}

        {state.isLoading ? <LoadingState /> : state.report && state.decision && !showEvidence ? (
          <PolicyDecisionView decision={state.decision} onNewReport={() => { setShowEvidence(false); state.startNew(); }} onReviewEvidence={() => setShowEvidence(true)} />
        ) : state.report?.status === 'CONFIRMED' && state.isClassifying ? (
          <ClassificationState report={state.report} />
        ) : state.report ? (
          <ReportWorkspace {...state} report={state.report} onShowPolicy={state.decision ? () => setShowEvidence(false) : undefined} />
        ) : (
          <div className="lab-onboarding">
            <DemoBrief />
            <IntakeForm context={state.context} busy={state.isSubmitting} onSubmit={state.upload} />
          </div>
        )}
      </main>
    </div>
  );
}

const AGENT_STEPS = [
  ['Reads', 'Laboratory report + policy'],
  ['Qualifies', 'Candidate fields + operating records'],
  ['Investigates', 'Mireye terrain + field boundaries'],
  ['Calculates', 'Conservative placement capacity'],
  ['Prepares', 'Cited professional handoff'],
];

function DemoBrief() {
  return (
    <aside className="demo-brief" aria-labelledby="demo-title">
      <div className="demo-brief__lead">
        <span className="demo-brief__mark"><RiShieldCheckLine aria-hidden="true" /></span>
        <div>
          <h1 id="demo-title">FieldProof</h1>
          <strong className="demo-brief__descriptor">Land-application evidence and placement agent</strong>
          <p>Combine Mireye terrain evidence with laboratory results, field boundaries, operating records, and application history before material is assigned.</p>
          <p className="demo-brief__buyer">For third-party land-application contractors and utilities that manage their own programs. PFAS is one batch input.</p>
          <div className="demo-brief__actions">
            <Button.Root asChild variant="neutral" mode="stroke" size="small"><a href="/judge-demo">View prepared case</a></Button.Root>
          </div>
        </div>
      </div>

      <ol className="agent-loop" aria-label="Agent decision loop">
        {AGENT_STEPS.map(([verb, object], index) => (
          <li key={object}>
            <span aria-hidden="true">{index + 1}</span>
            <div><strong>{verb}</strong><p>{object}</p></div>
          </li>
        ))}
      </ol>

      <details className="agent-loop-mobile">
        <summary>How FieldProof decides</summary>
        <ol>{AGENT_STEPS.map(([verb, object]) => <li key={object}><strong>{verb}</strong><span>{object}</span></li>)}</ol>
      </details>

      <p className="demo-brief__boundary">FieldProof prepares a calculation and evidence package. A responsible professional still authorizes application.</p>

      <details className="demo-brief__commercial">
        <summary>Buyer and pilot hypothesis</summary>
        <div className="demo-brief__proof">
          <div>
            <strong>Economic buyer</strong>
            <p>Contractor owner, operations manager, or utility biosolids program manager.</p>
          </div>
          <div>
            <strong>Daily user</strong>
            <p>Land-application coordinator, operator, or configured program reviewer.</p>
          </div>
          <div>
            <strong>Current alternative</strong>
            <p>Spreadsheets, GIS, email, paper agreements, consultant review, and MiEnviro.</p>
          </div>
          <div>
            <strong>Buying trigger</strong>
            <p>Field shortages, expiring records, new farms, repeated evidence follow-up, or a failed review.</p>
          </div>
          <div>
            <strong>Pilot</strong>
            <p>Reconstruct 20 historical placement decisions without influencing live applications.</p>
          </div>
          <div>
            <strong>Success measures</strong>
            <p>Missing evidence, reviewer agreement, preparation time, package completeness, and false-clear rate.</p>
          </div>
        </div>
        <p className="demo-brief__estimate"><strong>Status:</strong> labor cost, budget, pricing, pilot interest, and willingness to pay remain unvalidated. Unanswered outreach is not demand evidence.</p>
      </details>
    </aside>
  );
}

function IntakeForm({ context, busy, onSubmit }: {
  context: ReturnType<typeof useLabReport>['context'];
  busy: boolean;
  onSubmit: ReturnType<typeof useLabReport>['upload'];
}) {
  const [file, setFile] = useState<File | null>(null);
  const [facilityName, setFacilityName] = useState('');
  const [batchId, setBatchId] = useState('');
  const [wetMassKg, setWetMassKg] = useState('');
  const [percentSolids, setPercentSolids] = useState('');

  const selectedBatch = context?.batches?.find((batch) => batch.identifier === batchId);
  const selectedFacility = selectedBatch?.facilityName ?? facilityName;

  return (
    <section className="intake" id="evidence" aria-labelledby="page-title">
      <div className="page-header lab-page-header">
        <div>
          <h2 id="page-title">Add a PFAS lab report</h2>
          <p>Choose the tested batch and its original report. You’ll verify every extracted value before it is used.</p>
        </div>
      </div>

      <ol className="flow-steps" aria-label="Report flow">
        <li className="flow-steps__step flow-steps__step--current"><span>1</span>Upload report</li>
        <li className="flow-steps__step"><span>2</span>Verify values</li>
        <li className="flow-steps__step"><span>3</span>Get classification</li>
      </ol>

      <form className="intake-form" onSubmit={(event) => {
        event.preventDefault();
        if (!file || !selectedFacility || !batchId) return;
        void onSubmit({ facilityName: selectedFacility, batchId, wetMassKg, percentSolids, report: file });
      }}>
        <div className="form-field">
          <label htmlFor="facility">Facility</label>
          <input id="facility" list="facility-options" value={selectedFacility} onChange={(event) => {
            setFacilityName(event.target.value);
            setBatchId('');
          }} required placeholder="Start typing a facility name" />
          <datalist id="facility-options">
            {context?.facilities?.map((facility) => <option key={facility.id} value={facility.name} />)}
          </datalist>
        </div>

        <div className="form-field">
          <label htmlFor="batch">Batch</label>
          <input id="batch" list="batch-options" value={batchId} onChange={(event) => setBatchId(event.target.value)} required placeholder="Enter or choose a batch ID" />
          <datalist id="batch-options">
            {context?.batches?.filter((batch) => !facilityName || batch.facilityName === facilityName).map((batch) => (
              <option key={batch.id} value={batch.identifier}>{batch.facilityName}</option>
            ))}
          </datalist>
        </div>

        <div className="form-field report-field">
          <label htmlFor="report">Lab report</label>
          <label className={`file-picker${file ? ' file-picker--selected' : ''}`} htmlFor="report">
            <RiFileTextLine aria-hidden="true" />
            <span><strong>{file?.name ?? 'Choose a PDF, CSV, or JSON file'}</strong><small>{file ? formatBytes(file.size) : 'PDF, CSV, or JSON · up to 10 MB'}</small></span>
          </label>
          <input className="visually-hidden" id="report" type="file" accept=".pdf,.csv,.json,application/pdf,text/csv,application/json" onChange={(event) => setFile(event.target.files?.[0] ?? null)} required />
        </div>

        <div className="batch-details">
          <div>
            <strong>Batch details</strong>
            <small>Optional — used to compute the dry tons that need a field.</small>
          </div>
          <div className="optional-fields">
            <div className="form-field"><label htmlFor="mass">Wet mass <span>kg</span></label><input id="mass" type="number" min="0" step="any" value={wetMassKg} onChange={(event) => setWetMassKg(event.target.value)} /></div>
            <div className="form-field"><label htmlFor="solids">Total solids <span>%</span></label><input id="solids" type="number" min="0" max="100" step="any" value={percentSolids} onChange={(event) => setPercentSolids(event.target.value)} /></div>
          </div>
        </div>

        <Button.Root className="intake-submit" type="submit" variant="primary" mode="filled" disabled={busy || !file || !selectedFacility || !batchId}>
          {busy ? 'Uploading…' : 'Extract and review'}
        </Button.Root>
      </form>
    </section>
  );
}

function ReportWorkspace({ report, workspaceKey, isSubmitting, confirm, startNew, onShowPolicy }: {
  report: Report;
  workspaceKey: string;
  isSubmitting: boolean;
  confirm: ReturnType<typeof useLabReport>['confirm'];
  startNew: () => void;
  onShowPolicy?: () => void;
}) {
  const active = report.status === 'UPLOADED' || report.status === 'PROCESSING';
  const failed = report.status === 'FAILED';

  if (active) return <ProcessingState report={report} startNew={startNew} />;
  if (failed) return <FailedState report={report} startNew={startNew} />;
  if (!report.draft) return <FailedState report={report} startNew={startNew} />;

  return <EvidenceReview report={report} workspaceKey={workspaceKey} busy={isSubmitting} onConfirm={confirm} startNew={startNew} onShowPolicy={onShowPolicy} />;
}

function EvidenceReview({ report, workspaceKey, busy, onConfirm, startNew, onShowPolicy }: {
  report: Report;
  workspaceKey: string;
  busy: boolean;
  onConfirm: ReturnType<typeof useLabReport>['confirm'];
  startNew: () => void;
  onShowPolicy?: () => void;
}) {
  const initial = useMemo(() => correctionFromReport(report), [report]);
  const [form, setForm] = useState(initial);
  const [page, setPage] = useState(report.draft?.analytes?.[0]?.sourcePage ?? 1);
  const changed = JSON.stringify(form) !== JSON.stringify(initial);
  const confirmed = report.status === 'CONFIRMED';

  return (
    <section aria-labelledby="review-title">
      <div className="review-header">
        <div>
          <button className="back-button" type="button" onClick={startNew}><RiArrowLeftLine aria-hidden="true" /> New report</button>
          <h1 id="review-title">{report.batch.identifier}</h1>
          <p>{report.facility.name} · {report.originalFilename}</p>
        </div>
        {onShowPolicy ? <button className="text-button" type="button" onClick={onShowPolicy}>Back to classification</button> : <span className={`report-state report-state--${confirmed ? 'confirmed' : 'review'}`}>{confirmed ? <RiCheckLine aria-hidden="true" /> : null}{confirmed ? 'Confirmed' : `${report.gaps?.length ?? 0} item${(report.gaps?.length ?? 0) === 1 ? '' : 's'} to review`}</span>}
      </div>

      <div className="evidence-workspace">
        <SourcePane report={report} workspaceKey={workspaceKey} page={page} />
        <form className="evidence-form" onSubmit={(event) => { event.preventDefault(); void onConfirm(form, changed); }}>
          <div className="evidence-section evidence-metadata">
            <div className="evidence-section__head">
              <h2>Report details</h2>
              <span>From {report.originalFilename}</span>
            </div>
            <div className="field-grid">
              <TextField label="Laboratory" value={form.laboratory} onChange={(value) => setForm({ ...form, laboratory: value })} disabled={confirmed} />
              <TextField label="Sample ID" value={form.sampleIdentifier} onChange={(value) => setForm({ ...form, sampleIdentifier: value })} disabled={confirmed} />
              <TextField label="Collection date" type="date" value={form.collectionDate} onChange={(value) => setForm({ ...form, collectionDate: value })} disabled={confirmed} />
              <TextField label="Matrix" value={form.matrix} onChange={(value) => setForm({ ...form, matrix: value })} disabled={confirmed} />
              <TextField label="Method" value={form.method} onChange={(value) => setForm({ ...form, method: value })} disabled={confirmed} />
              <SelectField label="Report basis" value={form.basis ?? ''} options={[{ value: 'DRY', label: 'Dry weight' }, { value: 'WET', label: 'Wet weight' }]} onChange={(value) => setForm({ ...form, basis: value })} disabled={confirmed} />
            </div>
          </div>

          <div className="analyte-grid">
            {form.analytes?.map((analyte, index) => (
              <AnalyteEditor key={analyte.canonicalAnalyte} analyte={analyte} disabled={confirmed} onFocusPage={setPage} onChange={(next) => {
                const analytes = [...(form.analytes ?? [])] as [Analyte, Analyte];
                analytes[index] = next;
                setForm({ ...form, analytes });
              }} />
            ))}
          </div>

          {!confirmed ? (
            <div className="confirm-bar">
              <p>Confirm only after comparing both values with the original report.</p>
              <Button.Root type="submit" variant="primary" mode="filled" disabled={busy}>{busy ? 'Saving…' : 'Confirm values'}</Button.Root>
            </div>
          ) : null}
        </form>
      </div>
      <details className="report-details"><summary>File details</summary><dl><div><dt>File</dt><dd>{report.originalFilename}</dd></div><div><dt>Size</dt><dd>{formatBytes(report.sizeBytes)}</dd></div><div><dt>SHA-256</dt><dd>{report.sha256}</dd></div></dl></details>
    </section>
  );
}

function SourcePane({ report, workspaceKey, page }: { report: Report; workspaceKey: string; page: number }) {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let objectUrl: string | null = null;
    loadLabReportFile(workspaceKey, report.id, controller.signal).then((blob) => {
      objectUrl = URL.createObjectURL(blob);
      setUrl(objectUrl);
    }).catch(() => setUrl(null));
    return () => { controller.abort(); if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [report.id, workspaceKey]);
  const pageText = report.pages?.find((item) => item.number === page)?.text ?? '';
  const pageReadError = report.pages?.find((item) => item.number === page)?.readError ?? '';

  return (
    <details className="source-pane" open>
      <summary>Original report · page {page}</summary>
      <div className="source-document">
        {pageReadError ? <p className="source-read-error" role="alert">{pageReadError} The extracted values below may miss results shown on this page.</p> : null}
        {report.mediaType === 'application/pdf' && url ? <iframe title="Original lab report" src={`${url}#page=${page}&zoom=page-width`} /> : <pre>{pageText}</pre>}
      </div>
    </details>
  );
}

function AnalyteEditor({ analyte, disabled, onChange, onFocusPage }: { analyte: Analyte; disabled: boolean; onChange: (value: Analyte) => void; onFocusPage: (page: number) => void }) {
  const set = (field: keyof Analyte, value: string | number) => onChange({ ...analyte, [field]: value });
  return (
    <fieldset className="evidence-section analyte-section" disabled={disabled} onFocus={() => onFocusPage(analyte.sourcePage)}>
      <legend>{analyte.canonicalAnalyte}</legend>
      <div className="result-row">
        <TextField label="Result" value={analyte.resultText} onChange={(value) => set('resultText', value)} disabled={disabled} />
        <SelectField label="Unit" value={analyte.unit ?? ''} options={[{ value: 'UG_KG', label: 'µg/kg' }, { value: 'NG_G', label: 'ng/g' }, { value: 'MG_KG', label: 'mg/kg' }, { value: 'UG_L', label: 'µg/L' }]} onChange={(value) => set('unit', value)} disabled={disabled} />
        <SelectField label="Basis" value={analyte.basis ?? ''} options={[{ value: 'DRY', label: 'Dry' }, { value: 'WET', label: 'Wet' }]} onChange={(value) => set('basis', value)} disabled={disabled} />
      </div>
      <div className="field-grid analyte-details">
        <TextField label="Qualifier" value={analyte.qualifier} onChange={(value) => set('qualifier', value)} disabled={disabled} />
        <TextField label="Reporting limit" value={analyte.reportingLimit} onChange={(value) => set('reportingLimit', value)} disabled={disabled} />
        <TextField label="Detection limit" value={analyte.detectionLimit} onChange={(value) => set('detectionLimit', value)} disabled={disabled} />
      </div>
      <button className="source-excerpt" type="button" onClick={() => onFocusPage(analyte.sourcePage)}><span>Page {analyte.sourcePage}</span>{analyte.sourceExcerpt || 'No matching excerpt was found.'}</button>
      {analyte.normalizedValueUgKgDry ? <p className="normalized-value">Dry-weight value: {analyte.normalizedValueUgKgDry} µg/kg</p> : null}
    </fieldset>
  );
}

function TextField({ label, value, onChange, disabled, type = 'text' }: { label: string; value?: string; onChange: (value: string) => void; disabled: boolean; type?: string }) {
  const id = useId();
  return <div className="form-field"><label htmlFor={id}>{label}</label><input id={id} type={type} value={value ?? ''} onChange={(event) => onChange(event.target.value)} disabled={disabled} /></div>;
}

function SelectField({ label, value, options, onChange, disabled }: { label: string; value: string; options: Array<{ value: string; label: string }>; onChange: (value: string) => void; disabled: boolean }) {
  const id = useId();
  return <div className="form-field"><label htmlFor={id}>{label}</label><select id={id} value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled}><option value="">Choose</option>{options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></div>;
}

function LoadingState() { return <div className="center-state"><span className="state-spinner" /><h1>Loading lab evidence</h1></div>; }
function ClassificationState({ report }: { report: Report }) { return <div className="center-state"><span className="state-spinner" /><h1>Applying Michigan policy</h1><p>Comparing the confirmed PFOS and PFOA values with the active reviewed rule version for {report.batch.identifier}.</p></div>; }
function ProcessingState({ report, startNew }: { report: Report; startNew: () => void }) { return <div className="center-state"><span className="state-spinner" /><h1>Reading {report.originalFilename}</h1><p>Finding PFOS and PFOA values and linking them to the source pages.</p><button type="button" onClick={startNew}>Choose another report</button></div>; }
function FailedState({ report, startNew }: { report: Report; startNew: () => void }) { return <div className="center-state"><RiErrorWarningLine /><h1>We couldn’t read this report</h1><p>{failureCopy(report.failureCode)}</p><Button.Root variant="primary" mode="filled" onClick={startNew}>Choose another report</Button.Root></div>; }

function correctionFromReport(report: Report): CorrectionWritable {
  const analytes = report.draft?.analytes ?? [];
  const first = analytes[0];
  const second = analytes[1];
  return {
    laboratory: report.draft?.laboratory ?? '', sampleIdentifier: report.draft?.sampleIdentifier ?? '',
    collectionDate: report.draft?.collectionDate ?? '', matrix: report.draft?.matrix ?? '',
    method: report.draft?.method ?? '', basis: report.draft?.basis ?? '',
    analytes: first && second ? [first, second] : null,
  };
}

function failureCopy(code?: string) {
  if (code === 'UNSUPPORTED_FILE') return 'Use a PDF, CSV, or JSON report.';
  if (code === 'ENCRYPTED_PDF') return 'Remove the PDF password, then upload it again.';
  if (code === 'PAGE_LIMIT_EXCEEDED') return 'Use a report with 15 pages or fewer.';
  if (code === 'OCR_UNAVAILABLE') return 'This report is a scanned document, but OCR is not available on this server. Try a PDF with a text layer or export the results as CSV.';
  if (code === 'OCR_FAILED') return 'The scanned pages of this report could not be read. Try a clearer scan or export the results as CSV.';
  if (code === 'OCR_RENDER_FAILED') return 'The scanned pages of this report could not be prepared for reading. Try a clearer scan or export the results as CSV.';
  return 'Check that the file is readable, then upload it again.';
}

function formatBytes(bytes: number) { return bytes < 1_000_000 ? `${Math.ceil(bytes / 1_000)} KB` : `${(bytes / 1_000_000).toFixed(1)} MB`; }
