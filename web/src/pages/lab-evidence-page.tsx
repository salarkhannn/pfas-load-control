import { useEffect, useMemo, useState } from 'react';
import { RiArrowLeftLine, RiCheckLine, RiErrorWarningLine, RiFileTextLine } from '@remixicon/react';

import { loadLabReportFile } from '@/api';
import type { Analyte, CorrectionWritable, Report } from '@/client/types.gen';
import { PolicyDecisionView } from '@/components/policy-decision';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { useLabReport } from '@/hooks/use-lab-report';

export function LabEvidencePage() {
  const state = useLabReport();
  const [showEvidence, setShowEvidence] = useState(false);

  return (
    <div className="app-shell lab-shell">
      <header className="topbar lab-topbar">
        <a className="brand" href="/" aria-label="PFAS Load Control home">
          <span className="brand-mark" aria-hidden="true">
            {Array.from({ length: 9 }, (_, index) => <i key={index} />)}
          </span>
          <span>PFAS Load Control</span>
        </a>
      </header>

      <main className="workspace lab-workspace">
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
          <IntakeForm context={state.context} busy={state.isSubmitting} onSubmit={state.upload} />
        )}
      </main>
    </div>
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
    <section className="intake" aria-labelledby="page-title">
      <div className="page-header lab-page-header">
        <div>
          <p className="eyebrow">Lab evidence</p>
          <h1 id="page-title">Add a PFAS lab report</h1>
          <p>Choose the tested batch and upload the original report. You’ll verify every extracted value before it is used.</p>
        </div>
      </div>

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
            <span><strong>{file?.name ?? 'Choose a PDF, CSV, or JSON file'}</strong><small>{file ? formatBytes(file.size) : 'Up to 10 MB'}</small></span>
          </label>
          <input className="visually-hidden" id="report" type="file" accept=".pdf,.csv,.json,application/pdf,text/csv,application/json" onChange={(event) => setFile(event.target.files?.[0] ?? null)} required />
        </div>

        <details className="optional-details">
          <summary>Batch details</summary>
          <div className="optional-fields">
            <div className="form-field"><label htmlFor="mass">Wet mass <span>kg</span></label><input id="mass" inputMode="decimal" value={wetMassKg} onChange={(event) => setWetMassKg(event.target.value)} /></div>
            <div className="form-field"><label htmlFor="solids">Total solids <span>%</span></label><input id="solids" inputMode="decimal" value={percentSolids} onChange={(event) => setPercentSolids(event.target.value)} /></div>
          </div>
        </details>

        <Button.Root className="intake-submit" type="submit" variant="primary" mode="filled" disabled={busy || !file || !selectedFacility || !batchId}>
          {busy ? 'Uploading…' : 'Extract report'}
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
            <h2>Report details</h2>
            <div className="field-grid">
              <TextField label="Laboratory" value={form.laboratory} onChange={(value) => setForm({ ...form, laboratory: value })} disabled={confirmed} />
              <TextField label="Sample ID" value={form.sampleIdentifier} onChange={(value) => setForm({ ...form, sampleIdentifier: value })} disabled={confirmed} />
              <TextField label="Collection date" type="date" value={form.collectionDate} onChange={(value) => setForm({ ...form, collectionDate: value })} disabled={confirmed} />
              <TextField label="Matrix" value={form.matrix} onChange={(value) => setForm({ ...form, matrix: value })} disabled={confirmed} />
              <TextField label="Method" value={form.method} onChange={(value) => setForm({ ...form, method: value })} disabled={confirmed} />
              <SelectField label="Report basis" value={form.basis ?? ''} options={[{ value: 'DRY', label: 'Dry weight' }, { value: 'WET', label: 'Wet weight' }]} onChange={(value) => setForm({ ...form, basis: value })} disabled={confirmed} />
            </div>
          </div>

          {form.analytes?.map((analyte, index) => (
            <AnalyteEditor key={analyte.canonicalAnalyte} analyte={analyte} disabled={confirmed} onFocusPage={setPage} onChange={(next) => {
              const analytes = [...(form.analytes ?? [])] as [Analyte, Analyte];
              analytes[index] = next;
              setForm({ ...form, analytes });
            }} />
          ))}

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

  return (
    <details className="source-pane" open>
      <summary>Original report · page {page}</summary>
      <div className="source-document">
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
  return <div className="form-field"><label>{label}</label><input type={type} value={value ?? ''} onChange={(event) => onChange(event.target.value)} disabled={disabled} /></div>;
}

function SelectField({ label, value, options, onChange, disabled }: { label: string; value: string; options: Array<{ value: string; label: string }>; onChange: (value: string) => void; disabled: boolean }) {
  return <div className="form-field"><label>{label}</label><select value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled}><option value="">Choose</option>{options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></div>;
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
  return 'Check that the file is readable, then upload it again.';
}

function formatBytes(bytes: number) { return bytes < 1_000_000 ? `${Math.ceil(bytes / 1_000)} KB` : `${(bytes / 1_000_000).toFixed(1)} MB`; }
