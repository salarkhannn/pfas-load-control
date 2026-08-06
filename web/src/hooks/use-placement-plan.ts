import { useCallback, useEffect, useMemo, useState } from 'react';

import { createPlacementPlan, loadLatestPlacementPlan } from '@/api';
import type { PlacementPlan, PlanInputWritable } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

export function usePlacementPlan(decisionId: string) {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [plan, setPlan] = useState<PlacementPlan | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isBuilding, setIsBuilding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    loadLatestPlacementPlan(workspaceKey, decisionId, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setPlan(result);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(message(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });
    return () => controller.abort();
  }, [decisionId, workspaceKey]);

  const build = useCallback(async (input: PlanInputWritable = {}) => {
    setIsBuilding(true);
    setError(null);
    try {
      const result = await createPlacementPlan(workspaceKey, decisionId, input);
      setPlan(result);
      return result;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setIsBuilding(false);
    }
  }, [decisionId, workspaceKey]);

  return { plan, isLoading, isBuilding, error, build };
}

function message(reason: unknown): string {
  return reason instanceof Error ? reason.message : 'The fields could not be compared.';
}
