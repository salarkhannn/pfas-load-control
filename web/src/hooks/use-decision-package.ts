import { useCallback, useEffect, useMemo, useState } from 'react';

import { downloadDecisionPackage, generateDecisionPackage, loadLatestDecisionPackage } from '@/api';
import type { DecisionPackage } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

export function useDecisionPackage(decisionId: string) {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [value, setValue] = useState<DecisionPackage | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [busy, setBusy] = useState<'generate' | 'html' | 'pdf' | 'json' | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    loadLatestDecisionPackage(workspaceKey, decisionId, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setValue(result);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(message(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });
    return () => controller.abort();
  }, [decisionId, workspaceKey]);

  const generate = useCallback(async () => {
    setBusy('generate');
    setError(null);
    try {
      const result = await generateDecisionPackage(workspaceKey, decisionId);
      setValue(result);
      return result;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [decisionId, workspaceKey]);

  const download = useCallback(async (format: 'html' | 'pdf' | 'json') => {
    if (!value) return;
    setBusy(format);
    setError(null);
    try {
      await downloadDecisionPackage(workspaceKey, value.id, format);
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [value, workspaceKey]);

  return { value, isLoading, busy, error, generate, download };
}

function message(reason: unknown): string {
  return reason instanceof Error ? reason.message : 'The decision package could not be prepared.';
}
