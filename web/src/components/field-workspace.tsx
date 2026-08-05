import { useState } from 'react';
import {
  RiAddLine,
  RiCheckLine,
  RiErrorWarningLine,
  RiFileUploadLine,
  RiMapPin2Line,
} from '@remixicon/react';

import type { CreateInputWritable, DetailsInputWritable, Field } from '@/client/types.gen';
import { ParcelMap } from '@/components/parcel-map';
import { PhysicalEvidence } from '@/components/physical-evidence';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { useFieldWorkspace } from '@/hooks/use-field-workspace';
import { usePhysicalEvidence } from '@/hooks/use-physical-evidence';

const EMPTY_SAMPLE_POINTS: Array<{ index: number; label: string; latitude: number; longitude: number }> = [];

export function FieldWorkspace({ facilityName }: { facilityName?: string }) {
  const state = useFieldWorkspace(facilityName);
  const [adding, setAdding] = useState(false);
  const fields = state.context?.fields ?? [];

  return (
    <section className="field-workspace-panel" aria-labelledby="fields-title">
      <div className="field-page-header">
        <div>
          <h2 id="fields-title">Candidate fields</h2>
          <p>Confirm the field boundary and the application facts needed for screening.</p>
        </div>
        {fields.length > 0 && !adding ? (
          <Button.Root variant="primary" mode="filled" size="small" onClick={() => setAdding(true)}>
            <RiAddLine aria-hidden="true" /> Add field
          </Button.Root>
        ) : null}
      </div>

      {state.error ? (
        <Alert.Root className="error-alert field-error" variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Couldn’t update this field</strong><p>{state.error}</p></div>
        </Alert.Root>
      ) : null}

      {state.isLoading ? <FieldLoading /> : !state.context?.facilities?.length ? (
        <EmptyFacilities />
      ) : adding || fields.length === 0 ? (
        <AddField
          facilities={state.context.facilities}
          busy={state.busy === 'create' || state.busy === 'import'}
          onCancel={fields.length ? () => setAdding(false) : undefined}
          onCreate={async (facilityId, input) => { await state.create(facilityId, input); setAdding(false); }}
          onImport={state.importCSV}
        />
      ) : (
        <div className={`field-ledger${fields.length === 1 ? ' field-ledger--single' : ''}`}>
          <FieldInventory fields={fields} selectedId={state.selectedId} onSelect={state.select} onAdd={() => setAdding(true)} />
          {state.selected ? (
            <FieldRecord
              key={`${state.selected.id}-${state.selected.updatedAt}`}
              field={state.selected}
              busy={state.busy}
              onResolve={() => state.resolve(state.selected!.id)}
              onChoose={(index) => state.choose(state.selected!.id, index)}
              onConfirmParcel={() => state.confirmParcel(state.selected!.id)}
              onSetGeometry={(geojson) => state.setGeometry(state.selected!.id, geojson)}
              onSaveDetails={(details) => state.updateDetails(state.selected!.id, details)}
            />
          ) : <SelectFieldState />}
        </div>
      )}
    </section>
  );
}

function FieldInventory({ fields, selectedId, onSelect, onAdd }: {
  fields: Field[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onAdd: () => void;
}) {
  return (
    <aside className="field-inventory" aria-label="Candidate fields">
      <div className="field-inventory__head">
        <div><strong>Fields</strong><span>{fields.length}</span></div>
        <button type="button" onClick={onAdd} aria-label="Add field"><RiAddLine aria-hidden="true" /></button>
      </div>
      <div className="field-list">
        {fields.map((field) => (
          <button key={field.id} type="button" className={`field-list-row${field.id === selectedId ? ' field-list-row--selected' : ''}`} onClick={() => onSelect(field.id)}>
            <span className="field-list-row__icon"><RiMapPin2Line aria-hidden="true" /></span>
            <span className="field-list-row__copy"><strong>{field.name}</strong><small>{field.facility.name}</small></span>
            <FieldStatus status={field.status} compact />
          </button>
        ))}
      </div>
    </aside>
  );
}

function FieldRecord({ field, busy, onResolve, onChoose, onConfirmParcel, onSetGeometry, onSaveDetails }: {
  field: Field;
  busy: string | null;
  onResolve: () => Promise<unknown>;
  onChoose: (index: number) => Promise<unknown>;
  onConfirmParcel: () => Promise<unknown>;
  onSetGeometry: (geojson: string) => Promise<unknown>;
  onSaveDetails: (details: DetailsInputWritable) => Promise<unknown>;
}) {
  const physicalEvidence = usePhysicalEvidence(field.id, field.status === 'READY');

  return (
    <article className="field-record" aria-labelledby="field-title">
      <header className="field-record__header">
        <div>
          <p>{field.facility.name}</p>
          <h3 id="field-title">{field.name}</h3>
          <span>{field.gaps?.length ? `${field.gaps.length} item${field.gaps.length === 1 ? '' : 's'} needed` : 'Ready for screening'}</span>
        </div>
        <FieldStatus status={field.status} />
      </header>

      <div className="field-review-grid">
        <LocationSection field={field} samples={physicalEvidence.evaluation?.samples ?? EMPTY_SAMPLE_POINTS} busy={busy} onResolve={onResolve} onChoose={onChoose} onConfirmParcel={onConfirmParcel} onSetGeometry={onSetGeometry} />
        <FieldDetailsForm field={field} busy={busy === 'details'} onSave={onSaveDetails} />
      </div>
      {field.status === 'READY' ? (
        <PhysicalEvidence
          evaluation={physicalEvidence.evaluation}
          isLoading={physicalEvidence.isLoading}
          isStarting={physicalEvidence.isStarting}
          error={physicalEvidence.error}
          onStart={physicalEvidence.start}
        />
      ) : null}
    </article>
  );
}

function LocationSection({ field, samples, busy, onResolve, onChoose, onConfirmParcel, onSetGeometry }: {
  field: Field;
  samples: Array<{ index: number; label: string; latitude: number; longitude: number }>;
  busy: string | null;
  onResolve: () => Promise<unknown>;
  onChoose: (index: number) => Promise<unknown>;
  onConfirmParcel: () => Promise<unknown>;
  onSetGeometry: (geojson: string) => Promise<unknown>;
}) {
  const location = field.location;
  const candidates = location?.candidates ?? [];
  const displayedGeometry = field.geometry?.geojson ?? location?.parcel?.geometry;

  return (
    <section className="field-section boundary-section" aria-labelledby="location-title">
      <div className="field-section-heading">
        <div><h4 id="location-title">Boundary</h4><p>{field.geometry ? `${formatAcres(field.geometry.areaAcres)} acres confirmed` : field.locatorInput}</p></div>
        {field.geometry ? <span>Version {field.geometry.version}</span> : null}
      </div>

      {displayedGeometry ? (
        <ParcelMap
          geometry={displayedGeometry}
          latitude={location?.latitude}
          longitude={location?.longitude}
          label={location?.resolvedAddress || field.name}
          samples={samples}
        />
      ) : null}

      {field.geometry ? (
        <div className="boundary-confirmed">
          <span><RiCheckLine aria-hidden="true" /></span>
          <div><strong>Boundary confirmed</strong><p>{field.geometry.source === 'MIREYE_PARCEL_CONFIRMED' ? 'Mireye parcel confirmed by the operator' : 'Boundary supplied by the operator'}</p></div>
        </div>
      ) : location?.disposition === 'clarify' ? (
        <div className="location-choices">
          <p>Choose the matching location.</p>
          {candidates.map((candidate, index) => (
            <button key={`${candidate.latitude}-${candidate.longitude}`} type="button" onClick={() => void onChoose(index)} disabled={busy === 'choose'}>
              <span><strong>{candidate.resolvedAddress || candidate.label || `Location ${index + 1}`}</strong><small>{candidate.latitude}, {candidate.longitude}</small></span>
              <span>Choose</span>
            </button>
          ))}
        </div>
      ) : location?.disposition === 'resolved' ? (
        <div className="resolved-location">
          <dl>
            <div><dt>Matched location</dt><dd>{location.resolvedAddress || `${location.latitude}, ${location.longitude}`}</dd></div>
            <div><dt>County</dt><dd>{location.county || 'Not returned'}</dd></div>
            <div><dt>Confidence</dt><dd>{formatConfidence(location.confidence)}</dd></div>
            {location.parcel ? <div><dt>Parcel match</dt><dd>{parcelMatch(location.parcel.matchType, location.parcel.matchDistanceM)}</dd></div> : null}
          </dl>
          {location.parcel?.geometry ? (
            <div className="parcel-confirmation">
              <p>Compare the shaded parcel with the application field. Confirm only when the full outline matches.</p>
              <Button.Root variant="primary" mode="filled" size="small" disabled={busy === 'parcel'} onClick={() => void onConfirmParcel()}>
                {busy === 'parcel' ? 'Confirming…' : 'Confirm boundary'}
              </Button.Root>
            </div>
          ) : <p className="location-note">Mireye did not return a parcel boundary. Upload the actual field outline.</p>}
          <details className="alternate-boundary">
            <summary>Use a different boundary</summary>
            <BoundaryUpload busy={busy === 'geometry'} onUpload={onSetGeometry} />
          </details>
          <EvidenceFootnote field={field} />
        </div>
      ) : (
        <div className="unresolved-location">
          {location?.disposition === 'no_match' ? <div><strong>Location not matched</strong><p>{location.hint || readableNoMatch(location.reason)}</p></div> : <div><strong>Locate this field</strong><p>Use Mireye to match the address or coordinates.</p></div>}
          {field.locatorKind !== 'GEOJSON' ? <Button.Root variant="primary" mode="filled" size="small" onClick={() => void onResolve()} disabled={busy === 'resolve'}>{busy === 'resolve' ? 'Locating…' : location ? 'Try again' : 'Locate field'}</Button.Root> : null}
          <BoundaryUpload busy={busy === 'geometry'} onUpload={onSetGeometry} />
        </div>
      )}
    </section>
  );
}

function FieldDetailsForm({ field, busy, onSave }: { field: Field; busy: boolean; onSave: (details: DetailsInputWritable) => Promise<unknown> }) {
  const [form, setForm] = useState(() => detailsForm(field));
  const set = (name: keyof typeof form, value: string | boolean) => setForm((current) => ({ ...current, [name]: value }));

  return (
    <form className="field-section field-details" onSubmit={(event) => { event.preventDefault(); void onSave(compactDetails(form)); }}>
      <div className="field-section-heading"><div><h4>Application facts</h4><p>Enter values from the approved site and agronomic plan.</p></div></div>
      <div className="field-details-grid">
        <FieldInput label="MiEnviro site ID" value={form.miEnviroSiteId} onChange={(value) => set('miEnviroSiteId', value)} />
        <FieldInput label="Usable acres" value={form.usableAcres} onChange={(value) => set('usableAcres', value)} needed={hasGap(field, 'USABLE_ACRES_MISSING')} inputMode="decimal" />
        <FieldInput label="Agronomic rate" unit="dry tons/acre" value={form.agronomicRateDryTonsPerAcre} onChange={(value) => set('agronomicRateDryTonsPerAcre', value)} needed={hasGap(field, 'AGRONOMIC_RATE_MISSING')} inputMode="decimal" />
        <FieldInput label="Prior loading" unit="dry tons" value={form.priorLoadingDryTons} onChange={(value) => set('priorLoadingDryTons', value)} needed={hasGap(field, 'PRIOR_LOADING_MISSING')} inputMode="decimal" placeholder="Enter 0 if none" />
      </div>

      <div className={`rmp-confirmation${hasGap(field, 'RMP_APPROVAL_MISSING') ? ' rmp-confirmation--needed' : ''}`}>
        <label><input type="checkbox" checked={form.rmpApproved} onChange={(event) => set('rmpApproved', event.target.checked)} /><span><strong>Approved in the Residuals Management Program</strong><small>Confirm against the current approval record.</small></span></label>
        <input aria-label="RMP approval document or reference" placeholder="Approval document or reference" value={form.rmpDocumentReference} onChange={(event) => set('rmpDocumentReference', event.target.value)} />
      </div>

      <details className="optional-field-details">
        <summary>Crop and site notes</summary>
        <div className="field-details-grid">
          <label className="field-details-wide"><span>Crop or intended use</span><input value={form.cropOrUse} onChange={(event) => set('cropOrUse', event.target.value)} /></label>
          <label className="field-details-wide"><span>Wells, homes, water or easements</span><textarea rows={3} value={form.knownConstraints} onChange={(event) => set('knownConstraints', event.target.value)} /></label>
          <label className="field-details-wide"><span>Access constraints</span><textarea rows={3} value={form.accessConstraints} onChange={(event) => set('accessConstraints', event.target.value)} /></label>
        </div>
      </details>

      <Button.Root className="field-details-save" type="submit" variant="primary" mode="filled" size="small" disabled={busy}>{busy ? 'Saving…' : 'Save facts'}</Button.Root>
    </form>
  );
}

function FieldInput({ label, unit, value, onChange, needed, inputMode, placeholder }: {
  label: string;
  unit?: string;
  value: string;
  onChange: (value: string) => void;
  needed?: boolean;
  inputMode?: 'decimal';
  placeholder?: string;
}) {
  return (
    <label>
      <span>{label}{unit ? <small>{unit}</small> : null}{needed ? <em>Needed</em> : null}</span>
      <input inputMode={inputMode} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} />
    </label>
  );
}

function AddField({ facilities, busy, onCancel, onCreate, onImport }: {
  facilities: Array<{ id: string; name: string }>;
  busy: boolean;
  onCancel?: () => void;
  onCreate: (facilityId: string, input: CreateInputWritable) => Promise<unknown>;
  onImport: (facilityId: string, file: File) => Promise<unknown>;
}) {
  const [facilityId, setFacilityId] = useState(facilities[0]?.id ?? '');
  const [name, setName] = useState('');
  const [kind, setKind] = useState<CreateInputWritable['locatorKind']>('ADDRESS');
  const [locator, setLocator] = useState('');
  const [county, setCounty] = useState('');
  const [boundary, setBoundary] = useState<File | null>(null);
  const [csv, setCSV] = useState<File | null>(null);

  return (
    <section className="add-field" aria-labelledby="add-field-title">
      <div className="add-field__head"><div><h3 id="add-field-title">Add a field</h3><p>Start with the best address, coordinates, parcel number, or boundary you have.</p></div>{onCancel ? <button className="text-button" type="button" onClick={onCancel}>Cancel</button> : null}</div>
      <form className="add-field-form" onSubmit={async (event) => {
        event.preventDefault();
        let geojson: string | undefined;
        if (kind === 'GEOJSON') {
          if (!boundary) return;
          geojson = await boundary.text();
        }
        await onCreate(facilityId, { name, locatorKind: kind, locator: kind === 'GEOJSON' ? undefined : locator, county: kind === 'APN' ? county : undefined, geojson });
      }}>
        <div className={`add-field-row${facilities.length === 1 ? ' add-field-row--single' : ''}`}>
          {facilities.length > 1 ? <label><span>Facility</span><select value={facilityId} onChange={(event) => setFacilityId(event.target.value)}>{facilities.map((facility) => <option key={facility.id} value={facility.id}>{facility.name}</option>)}</select></label> : null}
          <label><span>Field name</span><input required maxLength={160} value={name} onChange={(event) => setName(event.target.value)} placeholder="DT01" /></label>
        </div>
        <fieldset className="locator-kind"><legend>Locate by</legend>{(['ADDRESS', 'COORDINATE', 'APN', 'GEOJSON'] as const).map((value) => <label key={value}><input type="radio" name="locator-kind" value={value} checked={kind === value} onChange={() => setKind(value)} /><span>{kindLabel(value)}</span></label>)}</fieldset>
        {kind === 'GEOJSON' ? (
          <label className={`boundary-file${boundary ? ' boundary-file--selected' : ''}`}><RiFileUploadLine aria-hidden="true" /><span><strong>{boundary?.name ?? 'Choose a boundary file'}</strong><small>Polygon or MultiPolygon GeoJSON · up to 1 MB</small></span><input className="visually-hidden" type="file" accept=".json,.geojson,application/geo+json,application/json" required onChange={(event) => setBoundary(event.target.files?.[0] ?? null)} /></label>
        ) : (
          <div className={`add-field-row${kind === 'APN' ? '' : ' add-field-row--single'}`}>
            <label><span>{kind === 'ADDRESS' ? 'Address' : kind === 'COORDINATE' ? 'Coordinates' : 'APN'}</span><input required value={locator} onChange={(event) => setLocator(event.target.value)} placeholder={kindPlaceholder(kind)} /></label>
            {kind === 'APN' ? <label><span>County</span><input required value={county} onChange={(event) => setCounty(event.target.value)} placeholder="Ingham" /></label> : null}
          </div>
        )}
        <Button.Root className="add-field-submit" type="submit" variant="primary" mode="filled" disabled={busy || !facilityId || !name || (kind === 'GEOJSON' ? !boundary : !locator)}>{busy ? 'Adding…' : 'Add field'}</Button.Root>
      </form>

      <details className="field-import">
        <summary>Import multiple fields</summary>
        <div><p>CSV columns: name, locator_kind, locator, county, geojson. Up to 25 fields.</p><label className="csv-picker"><input type="file" accept=".csv,text/csv" onChange={(event) => setCSV(event.target.files?.[0] ?? null)} /><span>{csv?.name ?? 'Choose CSV'}</span></label><Button.Root variant="neutral" mode="stroke" size="small" disabled={!csv || busy} onClick={() => { if (csv) void onImport(facilityId, csv); }}>Import fields</Button.Root></div>
      </details>
    </section>
  );
}

function BoundaryUpload({ busy, onUpload }: { busy: boolean; onUpload: (geojson: string) => Promise<unknown> }) {
  const [file, setFile] = useState<File | null>(null);
  return <div className="boundary-upload"><label><input type="file" accept=".json,.geojson,application/geo+json,application/json" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /><span>{file?.name ?? 'Choose boundary file'}</span></label><Button.Root variant="neutral" mode="stroke" size="small" disabled={!file || busy} onClick={async () => { if (file) await onUpload(await file.text()); }}>{busy ? 'Saving…' : 'Upload'}</Button.Root></div>;
}

function EvidenceFootnote({ field }: { field: Field }) {
  const location = field.location;
  if (!location) return null;
  return <p className="location-evidence">Mireye · {new Intl.DateTimeFormat('en-US', { dateStyle: 'medium' }).format(new Date(location.fetchedAt))}{location.requestId ? ` · ${location.requestId}` : ''}</p>;
}

function FieldStatus({ status, compact = false }: { status: string; compact?: boolean }) {
  const copy = status === 'READY' ? 'Ready' : status === 'NEEDS_LOCATION' ? 'Location needed' : status === 'NEEDS_GEOMETRY' ? 'Boundary needed' : 'Facts needed';
  return <span className={`field-status field-status--${status.toLowerCase()}${compact ? ' field-status--compact' : ''}`}>{status === 'READY' ? <RiCheckLine aria-hidden="true" /> : null}{copy}</span>;
}

function FieldLoading() { return <div className="center-state field-loading"><span className="state-spinner" /><h2>Loading fields</h2></div>; }
function SelectFieldState() { return <div className="field-record field-record--empty"><RiMapPin2Line aria-hidden="true" /><h3>Choose a field</h3><p>Select a field to review its boundary and application facts.</p></div>; }
function EmptyFacilities() { return <div className="field-empty"><RiMapPin2Line aria-hidden="true" /><h3>Add a lab report first</h3><p>A field belongs to the facility named on a lab report.</p></div>; }

function detailsForm(field: Field) {
  return {
    miEnviroSiteId: field.details.miEnviroSiteId ?? '', usableAcres: field.details.usableAcres ?? '',
    agronomicRateDryTonsPerAcre: field.details.agronomicRateDryTonsPerAcre ?? '', priorLoadingDryTons: field.details.priorLoadingDryTons ?? '',
    cropOrUse: field.details.cropOrUse ?? '', knownConstraints: field.details.knownConstraints ?? '', accessConstraints: field.details.accessConstraints ?? '',
    rmpApproved: field.details.rmpApproved ?? false, rmpDocumentReference: field.details.rmpDocumentReference ?? '',
  };
}

function compactDetails(form: ReturnType<typeof detailsForm>): DetailsInputWritable {
  const value = (input: string) => input.trim() || undefined;
  return {
    miEnviroSiteId: value(form.miEnviroSiteId), usableAcres: value(form.usableAcres),
    agronomicRateDryTonsPerAcre: value(form.agronomicRateDryTonsPerAcre), priorLoadingDryTons: value(form.priorLoadingDryTons),
    cropOrUse: value(form.cropOrUse), knownConstraints: value(form.knownConstraints), accessConstraints: value(form.accessConstraints),
    rmpApproved: form.rmpApproved, rmpDocumentReference: value(form.rmpDocumentReference),
  };
}

function hasGap(field: Field, code: string) { return field.gaps?.some((gap) => gap.code === code) ?? false; }
function kindLabel(kind: CreateInputWritable['locatorKind']) { return kind === 'ADDRESS' ? 'Address' : kind === 'COORDINATE' ? 'Coordinates' : kind === 'APN' ? 'APN' : 'Boundary file'; }
function kindPlaceholder(kind: CreateInputWritable['locatorKind']) { return kind === 'ADDRESS' ? 'Street, city, state' : kind === 'COORDINATE' ? '42.7335, -84.5555' : 'Parcel number'; }
function formatAcres(value: string) { const acres = Number(value); return Number.isFinite(acres) ? new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(acres) : value; }
function formatConfidence(value?: string) { const number = Number(value); return Number.isFinite(number) ? `${Math.round(number * 100)}%` : 'Not returned'; }
function parcelMatch(type?: string, distance?: string) { const label = type === 'exact_intersect' ? 'Exact point match' : type ? type.replaceAll('_', ' ') : 'Matched'; return distance && Number(distance) > 0 ? `${label} · ${Number(distance).toFixed(1)} m away` : label; }
function readableNoMatch(reason?: string) { return reason === 'apn_not_supported_in_v1' ? 'Use coordinates or upload the field boundary.' : 'Use a more precise locator or upload the field boundary.'; }
