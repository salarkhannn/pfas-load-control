import { useCallback, useEffect, useState } from 'react';

import type { Run } from '@/client/types.gen';
import { loadLatestRun, loadRun, startRun } from '@/api';

const activeStatuses = new Set(['QUEUED', 'RUNNING']);

export function useReadinessRun() {
  const [run, setRun] = useState<Run | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
	const runID = run?.id;
	const runStatus = run?.status;

  useEffect(() => {
    const controller = new AbortController();
    loadLatestRun(controller.signal)
      .then(setRun)
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setError(messageFrom(cause));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!runID || !runStatus || !activeStatuses.has(runStatus)) {
      return;
    }
    const controller = new AbortController();
    const timer = window.setInterval(() => {
      loadRun(runID, controller.signal)
        .then((nextRun) => {
          setRun(nextRun);
          setError(null);
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted) {
            setError(messageFrom(cause));
          }
        });
    }, 1_500);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [runID, runStatus]);

  const begin = useCallback(async () => {
    setIsStarting(true);
    setError(null);
    try {
      setRun(await startRun());
    } catch (cause: unknown) {
      setError(messageFrom(cause));
    } finally {
      setIsStarting(false);
    }
  }, []);

  return { run, isLoading, isStarting, error, begin };
}

function messageFrom(cause: unknown): string {
  return cause instanceof Error ? cause.message : 'Data access could not be checked.';
}
