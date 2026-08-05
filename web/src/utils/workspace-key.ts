const WORKSPACE_KEY_STORAGE = 'pfas.workspace-key.v1';
const REPORT_ID_STORAGE = 'pfas.current-report.v1';

export function getWorkspaceKey(): string {
  const stored = localStorage.getItem(WORKSPACE_KEY_STORAGE);
  if (stored) return stored;

  const bytes = crypto.getRandomValues(new Uint8Array(32));
  const key = btoa(String.fromCharCode(...bytes))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replaceAll('=', '');
  localStorage.setItem(WORKSPACE_KEY_STORAGE, key);
  return key;
}

export function getCurrentReportId(): string | null {
  return localStorage.getItem(REPORT_ID_STORAGE);
}

export function setCurrentReportId(id: string | null): void {
  if (id) localStorage.setItem(REPORT_ID_STORAGE, id);
  else localStorage.removeItem(REPORT_ID_STORAGE);
}
