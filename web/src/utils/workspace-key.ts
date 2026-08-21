const WORKSPACE_KEY_STORAGE = 'pfas.workspace-key.v1';
const REPORT_ID_STORAGE = 'pfas.current-report.v1';
const WORKSPACE_KEY_PATTERN = /^[A-Za-z0-9_-]{43,128}$/;

export function getWorkspaceKey(): string {
  const stored = localStorage.getItem(WORKSPACE_KEY_STORAGE);
  if (stored && WORKSPACE_KEY_PATTERN.test(stored)) return stored;

  if (stored) {
    localStorage.removeItem(WORKSPACE_KEY_STORAGE);
    localStorage.removeItem(REPORT_ID_STORAGE);
  }

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
