import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getCurrentReportId, getWorkspaceKey } from './workspace-key';

describe('workspace capability storage', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it('reuses a valid stored capability', () => {
    const stored = 'abcdefghijklmnopqrstuvwxyzABCDEFGH123456789';
    localStorage.setItem('pfas.workspace-key.v1', stored);

    expect(getWorkspaceKey()).toBe(stored);
  });

  it('replaces a malformed capability and clears its report pointer', () => {
    localStorage.setItem('pfas.workspace-key.v1', 'old-invalid-key');
    localStorage.setItem('pfas.current-report.v1', 'old-report-id');
    vi.spyOn(crypto, 'getRandomValues').mockImplementation((array) => {
      (array as Uint8Array).fill(7);
      return array;
    });

    const replacement = getWorkspaceKey();

    expect(replacement).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(replacement).not.toBe('old-invalid-key');
    expect(localStorage.getItem('pfas.workspace-key.v1')).toBe(replacement);
    expect(getCurrentReportId()).toBeNull();
  });
});
