import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  loadLatestPhysicalEvaluation,
  loadPhysicalEvaluation,
  startFieldPhysicalEvaluation,
} from '@/api';
import type { Evaluation } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

const ACTIVE_STATUSES = new Set(['QUEUED', 'RUNNING']);

export function usePhysicalEvidence(fieldId: string, enabled: boolean) {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [evaluation, setEvaluation] = useState<Evaluation | null>(null);
  const [isLoading, setIsLoading] = useState(enabled);
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const controller = new AbortController();
    loadLatestPhysicalEvaluation(workspaceKey, fieldId, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setEvaluation(result);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(message(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });
    return () => controller.abort();
  }, [enabled, fieldId, workspaceKey]);

  useEffect(() => {
    if (!evaluation || !ACTIVE_STATUSES.has(evaluation.status)) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      loadPhysicalEvaluation(workspaceKey, evaluation.id, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) {
            setEvaluation(result);
            setError(null);
          }
        })
        .catch((reason: unknown) => {
          if (!controller.signal.aborted) setError(message(reason));
        });
    }, 1_500);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [evaluation, workspaceKey]);

  const start = useCallback(async () => {
    setIsStarting(true);
    setError(null);
    try {
      const result = await startFieldPhysicalEvaluation(workspaceKey, fieldId);
      setEvaluation(result);
      return result;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setIsStarting(false);
    }
  }, [fieldId, workspaceKey]);

  return { evaluation, isLoading, isStarting, error, start };
}

function message(reason: unknown): string {
  return reason instanceof Error ? reason.message : 'The physical evidence could not be loaded.';
}
