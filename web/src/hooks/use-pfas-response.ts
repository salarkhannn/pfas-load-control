import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  confirmTreatmentPlantLocation,
  loadLatestPfasResponse,
  loadLatestResponseLocation,
  loadPfasResponse,
  resolveTreatmentPlantLocation,
  startPfasResponse,
} from '@/api';
import type { FacilityLocation, ResponseRun } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

const activeStatuses = new Set(['QUEUED', 'RUNNING']);

export function usePfasResponse(decisionId: string) {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [location, setLocation] = useState<FacilityLocation | null>(null);
  const [run, setRun] = useState<ResponseRun | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [busy, setBusy] = useState<'resolve' | 'start' | null>(null);
  const [error, setError] = useState<string | null>(null);
	const runId = run?.id;
	const runStatus = run?.status;

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      loadLatestPfasResponse(workspaceKey, decisionId, controller.signal),
      loadLatestResponseLocation(workspaceKey, decisionId, controller.signal),
    ]).then(([response, latestLocation]) => {
      if (controller.signal.aborted) return;
      setRun(response);
      setLocation(response?.location ?? latestLocation);
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(message(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setIsLoading(false);
    });
    return () => controller.abort();
  }, [decisionId, workspaceKey]);

  useEffect(() => {
		if (!runId || !runStatus || !activeStatuses.has(runStatus)) return;
    const controller = new AbortController();
    const timer = window.setInterval(() => {
			loadPfasResponse(workspaceKey, runId, controller.signal)
        .then((next) => {
          if (!controller.signal.aborted) setRun(next);
        })
        .catch((reason: unknown) => {
          if (!controller.signal.aborted) setError(message(reason));
        });
    }, 1500);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
	}, [runId, runStatus, workspaceKey]);

  const resolve = useCallback(async (kind: 'address' | 'coord', input: string) => {
    setBusy('resolve');
    setError(null);
    try {
      const next = await resolveTreatmentPlantLocation(workspaceKey, decisionId, { kind, input });
      setLocation(next);
      return next;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [decisionId, workspaceKey]);

  const confirmAndBuild = useCallback(async () => {
    if (!location) return;
    setBusy('start');
    setError(null);
    try {
      const confirmed = location.confirmed
        ? location
        : await confirmTreatmentPlantLocation(workspaceKey, location.id);
      setLocation(confirmed);
      const next = await startPfasResponse(workspaceKey, decisionId, confirmed.id);
      setRun(next);
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [decisionId, location, workspaceKey]);

  return {
    location,
    run,
    isLoading,
    busy,
    error,
    resolve,
    confirmAndBuild,
    clearError: () => setError(null),
  };
}

function message(reason: unknown): string {
  return reason instanceof Error ? reason.message : 'The required response could not be prepared.';
}
