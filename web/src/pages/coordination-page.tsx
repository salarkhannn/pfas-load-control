import { useEffect, useState } from 'react';
import {
  RiAddLine,
  RiArrowRightLine,
  RiCheckboxCircleLine,
  RiErrorWarningLine,
  RiLoader4Line,
  RiTeamLine,
  RiFileList3Line,
  RiMapPin2Line,
} from '@remixicon/react';

import {
  type Party,
  type CoordWorkflow,
  type AppRecord,
  type RegistryEntry,
  listParties,
  listWorkflows,
  listApplicationsByField,
  createParty,
  createWorkflow,
  createRegistryEntry,
  listRegistryEntries,
  searchRegistry,
  loadFieldContext,
} from '@/api';
import type { Field } from '@/client/types.gen';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { TopNav } from '@/components/top-nav';
import { getWorkspaceKey } from '@/utils/workspace-key';

type Tab = 'workflows' | 'parties' | 'registry' | 'applications';

const TAB_PARAMS: Record<string, Tab> = { parties: 'parties', registry: 'registry', applications: 'applications' };

function initialTab(): Tab {
  const param = new URLSearchParams(window.location.search).get('tab');
  return TAB_PARAMS[param ?? ''] ?? 'workflows';
}

const STATUS_LABELS: Record<string, string> = {
  NOT_STARTED: 'Not started',
  FARMER_CONFIRMED: 'Farmer confirmed',
  CONTRACTOR_CONFIRMED: 'Contractor confirmed',
  PLANT_CONFIRMED: 'Coordination complete',
  READY: 'Legacy coordination complete',
  REJECTED: 'Rejected',
};

const STATUS_COLOR: Record<string, string> = {
  NOT_STARTED: 'var(--fg-subtle)',
  FARMER_CONFIRMED: 'var(--state-warning)',
  CONTRACTOR_CONFIRMED: 'var(--state-info)',
  PLANT_CONFIRMED: 'var(--state-success)',
  READY: 'var(--state-success)',
  REJECTED: 'var(--state-danger)',
};

const ROLE_LABELS: Record<string, string> = {
  PLANT: 'Treatment Plant',
  CONTRACTOR: 'Contractor',
  FARMER: 'Farmer',
};

const ROLE_ICON: Record<string, string> = {
  PLANT: '🏭',
  CONTRACTOR: '🚛',
  FARMER: '🌾',
};

export function CoordinationPage() {
  const [tab, setTabState] = useState<Tab>(initialTab);
  const [showNewParty, setShowNewParty] = useState(false);
  const [showNewWorkflow, setShowNewWorkflow] = useState(false);
  const [showNewRegistryEntry, setShowNewRegistryEntry] = useState(false);

  const setTab = (next: Tab) => {
    setTabState(next);
    const url = new URL(window.location.href);
    if (next === 'workflows') url.searchParams.delete('tab');
    else url.searchParams.set('tab', next);
    window.history.replaceState(null, '', url);
  };

  return (
    <div className="app-shell">
      <TopNav />
      <header className="coord-tabs">
        <nav className="coord-nav" aria-label="Coordination sections">
          <a href="/coordination" aria-current={tab === 'workflows' ? 'page' : undefined} className={`coord-tab${tab === 'workflows' ? ' coord-tab--active' : ''}`} onClick={(event) => { event.preventDefault(); setTab('workflows'); }}>
            <RiFileList3Line aria-hidden="true" /> Workflows
          </a>
          <a href="/coordination?tab=parties" aria-current={tab === 'parties' ? 'page' : undefined} className={`coord-tab${tab === 'parties' ? ' coord-tab--active' : ''}`} onClick={(event) => { event.preventDefault(); setTab('parties'); }}>
            <RiTeamLine aria-hidden="true" /> Parties
          </a>
          <a href="/coordination?tab=registry" aria-current={tab === 'registry' ? 'page' : undefined} className={`coord-tab${tab === 'registry' ? ' coord-tab--active' : ''}`} onClick={(event) => { event.preventDefault(); setTab('registry'); }}>
            <RiMapPin2Line aria-hidden="true" /> Field Registry
          </a>
          <a href="/coordination?tab=applications" aria-current={tab === 'applications' ? 'page' : undefined} className={`coord-tab${tab === 'applications' ? ' coord-tab--active' : ''}`} onClick={(event) => { event.preventDefault(); setTab('applications'); }}>
            <RiCheckboxCircleLine aria-hidden="true" /> Applications
          </a>
        </nav>
      </header>

      <main className="workspace page-content coord-workspace">
        {tab === 'workflows' && (
          <WorkflowPanel
            showNew={showNewWorkflow}
            onToggleNew={() => setShowNewWorkflow(!showNewWorkflow)}
          />
        )}
        {tab === 'parties' && (
          <PartyPanel
            showNew={showNewParty}
            onToggleNew={() => setShowNewParty(!showNewParty)}
          />
        )}
        {tab === 'registry' && (
          <RegistryPanel
            showNew={showNewRegistryEntry}
            onToggleNew={() => setShowNewRegistryEntry(!showNewRegistryEntry)}
          />
        )}
        {tab === 'applications' && <ApplicationPanel />}
      </main>
    </div>
  );
}

// ─── Workflows ────────────────────────────────────────────────────────

function WorkflowPanel({ showNew, onToggleNew }: {
  showNew: boolean;
  onToggleNew: () => void;
}) {
  const ws = getWorkspaceKey();
  const [workflows, setWorkflows] = useState<CoordWorkflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const next = await listWorkflows(ws);
        if (!active) return;
        setWorkflows(next);
        setError(null);
      } catch (e) {
        if (!active) return;
        setError(errorMessage(e, 'The project server could not be reached. Check the connection, then try again.'));
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [ws]);

  const handleCreated = (created: CoordWorkflow) => {
    setWorkflows((prev) => [created, ...prev]);
    onToggleNew();
  };

  return (
    <section className="coord-panel" aria-labelledby="wf-title">
      <div className="page-header">
        <div>
          <h1 id="wf-title">Coordination Workflows</h1>
          <p>Document farmer, contractor, and plant coordination without implying agronomic approval.</p>
        </div>
        <Button.Root variant="primary" mode="filled" size="small" onClick={onToggleNew}>
          <RiAddLine aria-hidden="true" /> New workflow
        </Button.Root>
      </div>

      {error && (
        <Alert.Root variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Error</strong><p>{error}</p></div>
        </Alert.Root>
      )}

      {showNew && <NewWorkflowForm ws={ws} onCreated={handleCreated} onCancel={onToggleNew} />}

      {loading ? (
        <div className="coord-loading"><RiLoader4Line className="spin" aria-hidden="true" /> Loading workflows...</div>
      ) : error ? null : workflows.length === 0 ? (
        <div className="coord-empty">
          <RiFileList3Line aria-hidden="true" />
          <p>No workflows yet. Create one to start coordinating a field application.</p>
        </div>
      ) : (
        <div className="coord-list">
          {workflows.map((wf) => (
            <a key={wf.id} href={`/coordination/workflow/${wf.id}`} className="coord-card">
              <div className="coord-card__header">
                <span className="coord-card__field">{wf.fieldName || 'Unnamed field'}</span>
                <span className="coord-card__status" style={{ color: STATUS_COLOR[wf.status] || 'var(--fg-subtle)' }}>
                  {STATUS_LABELS[wf.status] || wf.status}
                </span>
              </div>
              <div className="coord-card__meta">
                <span>Started by {wf.createdByName || 'Unknown'}</span>
                <span>{formatDate(wf.createdAt)}</span>
              </div>
              <RiArrowRightLine className="coord-card__arrow" aria-hidden="true" />
            </a>
          ))}
        </div>
      )}
    </section>
  );
}

function NewWorkflowForm({ ws, onCreated, onCancel }: { ws: string; onCreated: (workflow: CoordWorkflow) => void; onCancel: () => void }) {
  const [parties, setParties] = useState<Party[]>([]);
  const [fields, setFields] = useState<Field[]>([]);
  const [plantId, setPlantId] = useState('');
  const [fieldId, setFieldId] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([listParties(ws, 'PLANT'), loadFieldContext(ws)])
      .then(([nextParties, context]) => { setParties(nextParties); setFields(context.fields ?? []); })
      .catch(() => setError('Could not load the plants and candidate fields needed to create a workflow.'));
  }, [ws]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!plantId || !fieldId) return;
    setBusy(true);
    setError(null);
    try {
      const created = await createWorkflow(ws, { createdByPartyId: plantId, fieldId });
      onCreated(created);
    } catch (err) {
      setError(errorMessage(err, 'Could not create the workflow.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="coord-form" onSubmit={handleSubmit}>
      <h3>New coordination workflow</h3>
      <div className="form-field">
        <label htmlFor="wf-plant">Plant</label>
        <select id="wf-plant" value={plantId} onChange={(e) => setPlantId(e.target.value)} required disabled={parties.length === 0}>
          <option value="">{parties.length === 0 ? 'No treatment plant party yet' : 'Select a plant...'}</option>
          {parties.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        {parties.length === 0 && <small className="field-hint">Add a party with role “Treatment Plant” in the Parties tab first.</small>}
      </div>
      <div className="form-field">
        <label htmlFor="wf-field">Candidate field</label>
        <select id="wf-field" value={fieldId} onChange={(e) => setFieldId(e.target.value)} required disabled={fields.length === 0}>
          <option value="">{fields.length === 0 ? 'No candidate fields available' : 'Select a field…'}</option>
          {fields.map((field) => <option key={field.id} value={field.id}>{field.name} · {field.facility.name} · {field.status.replaceAll('_', ' ').toLowerCase()}</option>)}
        </select>
        {fields.length === 0 && <small className="field-hint">Add and verify a candidate field from the lab evidence flow before coordinating it.</small>}
      </div>
      {error && <Alert.Root variant="lighter" status="error" size="large" role="alert"><Alert.Icon as={RiErrorWarningLine} /><p>{error}</p></Alert.Root>}
      <div className="coord-form__actions">
        <Button.Root type="submit" variant="primary" mode="filled" size="small" disabled={busy || !plantId || !fieldId}>
          {busy ? 'Creating...' : 'Create workflow'}
        </Button.Root>
        <Button.Root type="button" variant="neutral" mode="ghost" size="small" onClick={onCancel}>Cancel</Button.Root>
      </div>
    </form>
  );
}

// ─── Parties ──────────────────────────────────────────────────────────

function PartyPanel({ showNew, onToggleNew }: { showNew: boolean; onToggleNew: () => void }) {
  const ws = getWorkspaceKey();
  const [parties, setParties] = useState<Party[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<string>('');

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const next = await listParties(ws, filter || undefined);
        if (!active) return;
        setParties(next);
        setError(null);
      } catch (e) {
        if (!active) return;
        setError(errorMessage(e, 'Could not load parties.'));
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [ws, filter]);

  const handleCreate = async (data: { role: string; name: string; email: string; phone?: string }) => {
    const created = await createParty(ws, data);
    setParties((prev) => [created, ...prev]);
  };

  return (
    <section className="coord-panel" aria-labelledby="party-title">
      <div className="page-header">
        <div>
          <h1 id="party-title">Parties</h1>
          <p>Plants, contractors, and farmers involved in biosolids application.</p>
        </div>
        <div className="page-header__actions">
          <select className="coord-filter" value={filter} onChange={(e) => setFilter(e.target.value)}>
            <option value="">All roles</option>
            <option value="PLANT">Plants</option>
            <option value="CONTRACTOR">Contractors</option>
            <option value="FARMER">Farmers</option>
          </select>
          <Button.Root variant="primary" mode="filled" size="small" onClick={onToggleNew}>
            <RiAddLine aria-hidden="true" /> Add party
          </Button.Root>
        </div>
      </div>

      {error && (
        <Alert.Root variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Error</strong><p>{error}</p></div>
        </Alert.Root>
      )}

      {showNew && <NewPartyForm onCreated={handleCreate} onCancel={onToggleNew} />}

      {loading ? (
        <div className="coord-loading"><RiLoader4Line className="spin" aria-hidden="true" /> Loading parties...</div>
      ) : error ? null : parties.length === 0 ? (
        <div className="coord-empty">
          <RiTeamLine aria-hidden="true" />
          <p>No parties added yet. Add a plant, contractor, or farmer to get started.</p>
        </div>
      ) : (
        <div className="coord-list">
          {parties.map((p) => (
            <div key={p.id} className="coord-card coord-card--party">
              <span className="coord-card__icon" aria-hidden="true">{ROLE_ICON[p.role]}</span>
              <div className="coord-card__body">
                <span className="coord-card__name">{p.name}</span>
                <span className="coord-card__role">{ROLE_LABELS[p.role]}</span>
                <span className="coord-card__email">{p.email}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function NewPartyForm({ onCreated, onCancel }: { onCreated: (data: { role: string; name: string; email: string; phone?: string }) => void; onCancel: () => void }) {
  const [role, setRole] = useState('FARMER');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await onCreated({ role, name, email, phone });
      onCancel();
    } catch (err) {
      setError(errorMessage(err, 'Could not create the party.'));
      setBusy(false);
    }
  };

  return (
    <form className="coord-form" onSubmit={handleSubmit}>
      <h3>Add a party</h3>
      <div className="form-row">
        <div className="form-field">
          <label htmlFor="party-role">Role</label>
          <select id="party-role" value={role} onChange={(e) => setRole(e.target.value)} required>
            <option value="PLANT">Treatment Plant</option>
            <option value="CONTRACTOR">Contractor</option>
            <option value="FARMER">Farmer</option>
          </select>
        </div>
        <div className="form-field">
          <label htmlFor="party-name">Name</label>
          <input id="party-name" value={name} onChange={(e) => setName(e.target.value)} required placeholder="e.g. GLWWTP" />
        </div>
      </div>
      <div className="form-row">
        <div className="form-field">
          <label htmlFor="party-email">Email <small>(optional)</small></label>
          <input id="party-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="ops@glw.org" />
        </div>
        <div className="form-field">
          <label htmlFor="party-phone">Phone (optional)</label>
          <input id="party-phone" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="(517) 555-0123" />
        </div>
      </div>
      {error && <Alert.Root variant="lighter" status="error" size="large" role="alert"><Alert.Icon as={RiErrorWarningLine} /><p>{error}</p></Alert.Root>}
      <div className="coord-form__actions">
        <Button.Root type="submit" variant="primary" mode="filled" size="small" disabled={busy}>
          {busy ? 'Adding...' : 'Add party'}
        </Button.Root>
        <Button.Root type="button" variant="neutral" mode="ghost" size="small" onClick={onCancel}>Cancel</Button.Root>
      </div>
    </form>
  );
}

// ─── Registry ─────────────────────────────────────────────────────────

function RegistryPanel({ showNew, onToggleNew }: { showNew: boolean; onToggleNew: () => void }) {
  const ws = getWorkspaceKey();
  const [entries, setEntries] = useState<RegistryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState('');

  useEffect(() => {
    let active = true;
    const timer = window.setTimeout(() => void (async () => {
      try {
        const found = search.trim()
          ? await searchRegistry(ws, search)
          : await listRegistryEntries(ws, filter || undefined);
        const next = search.trim() && filter ? found.filter((entry) => entry.entryType === filter) : found;
        if (!active) return;
        setEntries(next);
        setError(null);
      } catch (e) {
        if (!active) return;
        setError(errorMessage(e, 'Could not load the field registry.'));
      } finally {
        if (active) setLoading(false);
      }
    })(), search.trim() ? 250 : 0);
    return () => { active = false; window.clearTimeout(timer); };
  }, [ws, search, filter]);

  const handleCreate = async (data: { entryType: string; name: string; data?: Record<string, unknown>; latitude?: number; longitude?: number }) => {
    const created = await createRegistryEntry(ws, data);
    setEntries((prev) => [created, ...prev]);
  };

  return (
    <section className="coord-panel" aria-labelledby="reg-title">
      <div className="page-header">
        <div>
          <h1 id="reg-title">Field Registry</h1>
          <p>Discover and manage fields, plants, and contractors across Michigan.</p>
        </div>
        <Button.Root variant="primary" mode="filled" size="small" onClick={onToggleNew}>
          <RiAddLine aria-hidden="true" /> Add entry
        </Button.Root>
      </div>

      <div className="coord-search">
        <label className="visually-hidden" htmlFor="registry-search">Search registry</label>
        <input
          id="registry-search"
          className="coord-search__input"
          type="search"
          placeholder="Search fields, plants, contractors..."
          value={search}
          onChange={(e) => { setLoading(true); setSearch(e.target.value); }}
        />
        <label className="visually-hidden" htmlFor="registry-type">Filter registry by type</label>
        <select id="registry-type" className="coord-filter" value={filter} onChange={(e) => { setLoading(true); setFilter(e.target.value); }}>
          <option value="">All types</option>
          <option value="FIELD">Fields</option>
          <option value="PLANT">Plants</option>
          <option value="CONTRACTOR">Contractors</option>
        </select>
      </div>

      {error && (
        <Alert.Root variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Error</strong><p>{error}</p></div>
        </Alert.Root>
      )}

      {showNew && <NewRegistryEntryForm onCreated={handleCreate} onCancel={onToggleNew} />}

      {loading ? (
        <div className="coord-loading"><RiLoader4Line className="spin" aria-hidden="true" /> Loading registry...</div>
      ) : error ? null : entries.length === 0 ? (
        <div className="coord-empty">
          <RiMapPin2Line aria-hidden="true" />
          <p>No entries found. Add a field, plant, or contractor to the registry.</p>
        </div>
      ) : (
        <div className="coord-list">
          {entries.map((e) => (
            <div key={e.id} className="coord-card coord-card--registry">
              <div className="coord-card__header">
                <span className="coord-card__name">{e.name}</span>
                <span className="coord-card__badge">{e.entryType}</span>
              </div>
              {e.latitude && e.longitude && (
                <span className="coord-card__meta">{e.latitude.toFixed(4)}, {e.longitude.toFixed(4)}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function NewRegistryEntryForm({ onCreated, onCancel }: { onCreated: (data: { entryType: string; name: string; data?: Record<string, unknown>; latitude?: number; longitude?: number }) => void; onCancel: () => void }) {
  const [entryType, setEntryType] = useState('FIELD');
  const [name, setName] = useState('');
  const [lat, setLat] = useState('');
  const [lng, setLng] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      onCreated({
        entryType,
        name,
        data: {},
        ...(lat && lng ? { latitude: parseFloat(lat), longitude: parseFloat(lng) } : {}),
      });
      onCancel();
    } catch (err) {
      setError(errorMessage(err, 'Could not create the registry entry.'));
      setBusy(false);
    }
  };

  return (
    <form className="coord-form" onSubmit={handleSubmit}>
      <h3>Add registry entry</h3>
      <div className="form-row">
        <div className="form-field">
          <label htmlFor="reg-type">Type</label>
          <select id="reg-type" value={entryType} onChange={(e) => setEntryType(e.target.value)} required>
            <option value="FIELD">Field</option>
            <option value="PLANT">Plant</option>
            <option value="CONTRACTOR">Contractor</option>
          </select>
        </div>
        <div className="form-field">
          <label htmlFor="reg-name">Name</label>
          <input id="reg-name" value={name} onChange={(e) => setName(e.target.value)} required placeholder="e.g. Decker North 40" />
        </div>
      </div>
      <div className="form-row">
        <div className="form-field">
          <label htmlFor="reg-lat">Latitude</label>
          <input id="reg-lat" type="number" step="any" value={lat} onChange={(e) => setLat(e.target.value)} placeholder="42.9" />
        </div>
        <div className="form-field">
          <label htmlFor="reg-lng">Longitude</label>
          <input id="reg-lng" type="number" step="any" value={lng} onChange={(e) => setLng(e.target.value)} placeholder="-84.2" />
        </div>
      </div>
      {error && <Alert.Root variant="lighter" status="error" size="large" role="alert"><Alert.Icon as={RiErrorWarningLine} /><p>{error}</p></Alert.Root>}
      <div className="coord-form__actions">
        <Button.Root type="submit" variant="primary" mode="filled" size="small" disabled={busy}>
          {busy ? 'Adding...' : 'Add entry'}
        </Button.Root>
        <Button.Root type="button" variant="neutral" mode="ghost" size="small" onClick={onCancel}>Cancel</Button.Root>
      </div>
    </form>
  );
}

// ─── Applications ─────────────────────────────────────────────────────

function ApplicationPanel() {
  const ws = getWorkspaceKey();
  const [records, setRecords] = useState<AppRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [fieldFilter, setFieldFilter] = useState('');
  const [fields, setFields] = useState<Field[]>([]);

  useEffect(() => {
    const controller = new AbortController();
    loadFieldContext(ws, undefined, controller.signal)
      .then((context) => setFields(context.fields ?? []))
      .catch((reason: unknown) => { if (!controller.signal.aborted) setError(errorMessage(reason, 'Could not load candidate fields.')); });
    return () => controller.abort();
  }, [ws]);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const next = fieldFilter ? await listApplicationsByField(ws, fieldFilter) : [];
        if (!active) return;
        setRecords(next);
        setError(null);
      } catch (e) {
        if (!active) return;
        setError(errorMessage(e, 'Could not load application history.'));
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [ws, fieldFilter]);

  return (
    <section className="coord-panel" aria-labelledby="app-title">
      <div className="page-header">
        <div>
          <h1 id="app-title">Applications</h1>
          <p>Review recorded loading history for a verified candidate field.</p>
        </div>
      </div>

      <div className="coord-search">
        <label className="visually-hidden" htmlFor="application-field">Candidate field</label>
        <select
          id="application-field"
          className="coord-search__input"
          value={fieldFilter}
          onChange={(e) => { setLoading(true); setFieldFilter(e.target.value); }}
        >
          <option value="">Select a candidate field…</option>
          {fields.map((field) => <option key={field.id} value={field.id}>{field.name} · {field.facility.name}</option>)}
        </select>
      </div>

      {error && (
        <Alert.Root variant="lighter" status="error" size="large" role="alert">
          <Alert.Icon as={RiErrorWarningLine} />
          <div><strong>Error</strong><p>{error}</p></div>
        </Alert.Root>
      )}

      {loading ? (
        <div className="coord-loading"><RiLoader4Line className="spin" aria-hidden="true" /> Loading...</div>
      ) : error ? null : records.length === 0 ? (
        <div className="coord-empty">
          <RiCheckboxCircleLine aria-hidden="true" />
          <p>{fieldFilter ? 'No applications are recorded for this field.' : 'Select a field to view its application history.'}</p>
        </div>
      ) : (
        <div className="coord-list">
          {records.map((r) => (
            <div key={r.id} className="coord-card coord-card--app">
              <div className="coord-card__header">
                <span className="coord-card__name">{r.fieldName || r.fieldId}</span>
                <span className="coord-card__date">{r.applicationDate}</span>
              </div>
              <div className="coord-card__meta">
                <span>{r.dryTons} dry tons</span>
                <span>{r.acresApplied} acres</span>
                <span>{r.contractorName || r.contractorId}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  } catch {
    return iso;
  }
}

function errorMessage(error: unknown, fallback: string): string {
  if (!(error instanceof Error) || error.message === 'Failed to fetch' || error.message === 'Load failed') return fallback;
  return error.message;
}
