import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  addCandidateField,
  chooseFieldLocation,
  confirmUploadedBoundary,
  confirmParcelBoundary,
  importFieldCSV,
  loadFieldContext,
  resolveFieldLocation,
  saveFieldDetails,
  saveFieldGeometry,
} from '@/api';
import type { CreateInputWritable, DetailsInputWritable, Field, FieldContext, Import } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

export function useFieldWorkspace(facilityName?: string) {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [context, setContext] = useState<FieldContext | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(() => new URLSearchParams(window.location.search).get('field'));
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    const next = await loadFieldContext(workspaceKey, undefined, signal);
    setContext(next);
    setSelectedId((current) => chooseSelectedId(next, current, facilityName));
  }, [facilityName, workspaceKey]);

  useEffect(() => {
    const controller = new AbortController();
    loadFieldContext(workspaceKey, undefined, controller.signal)
      .then((next) => {
        if (controller.signal.aborted) return;
        setContext(next);
        setSelectedId((current) => chooseSelectedId(next, current, facilityName));
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(message(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });
    return () => controller.abort();
  }, [facilityName, workspaceKey]);

  const replace = useCallback((field: Field) => {
    setContext((current) => current ? {
      ...current,
      fields: current.fields?.some((item) => item.id === field.id)
        ? current.fields.map((item) => item.id === field.id ? field : item)
        : [field, ...(current.fields ?? [])],
    } : current);
    setSelectedId(field.id);
  }, []);

  const act = useCallback(async (label: string, action: () => Promise<Field>) => {
    setBusy(label);
    setError(null);
    try {
      const field = await action();
      replace(field);
      return field;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [replace]);

  const create = useCallback(async (facilityId: string, input: CreateInputWritable) => {
    setBusy('create');
    setError(null);
    let created: Field | null = null;
    try {
      created = await addCandidateField(workspaceKey, facilityId, input);
      replace(created);
      if (input.locatorKind !== 'GEOJSON') {
        const resolved = await resolveFieldLocation(workspaceKey, created.id);
        replace(resolved);
        return resolved;
      }
      return created;
    } catch (reason) {
      if (created) await refresh().catch(() => undefined);
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [refresh, replace, workspaceKey]);

  const importCSV = useCallback(async (facilityId: string, file: File): Promise<Import> => {
    setBusy('import');
    setError(null);
    try {
      const result = await importFieldCSV(workspaceKey, facilityId, file);
      await refresh();
      return result;
    } catch (reason) {
      setError(message(reason));
      throw reason;
    } finally {
      setBusy(null);
    }
  }, [refresh, workspaceKey]);

  const select = useCallback((id: string | null) => {
    setSelectedId(id);
    const url = new URL(window.location.href);
    if (id) url.searchParams.set('field', id); else url.searchParams.delete('field');
    window.history.replaceState(null, '', url);
  }, []);

  const visibleContext = context ? {
    ...context,
    facilities: context.facilities?.filter((facility) => !facilityName || facility.name === facilityName),
    fields: context.fields?.filter((field) => !facilityName || field.facility.name === facilityName),
  } : null;

  return {
    context: visibleContext,
    selected: visibleContext?.fields?.find((field) => field.id === selectedId) ?? null,
    selectedId,
    isLoading,
    busy,
    error,
    clearError: () => setError(null),
    select,
    create,
    importCSV,
    resolve: (id: string) => act('resolve', () => resolveFieldLocation(workspaceKey, id)),
    choose: (id: string, index: number) => act('choose', () => chooseFieldLocation(workspaceKey, id, index)),
    confirmParcel: (id: string) => act('parcel', () => confirmParcelBoundary(workspaceKey, id)),
    confirmGeometry: (id: string) => act('boundary-confirmation', () => confirmUploadedBoundary(workspaceKey, id)),
    setGeometry: (id: string, geojson: string) => act('geometry', () => saveFieldGeometry(workspaceKey, id, geojson)),
    updateDetails: (id: string, details: DetailsInputWritable) => act('details', () => saveFieldDetails(workspaceKey, id, details)),
  };
}

function chooseSelectedId(context: FieldContext, current: string | null, facilityName?: string): string | null {
  const fields = context.fields?.filter((field) => !facilityName || field.facility.name === facilityName) ?? [];
  return fields.some((field) => field.id === current) ? current : fields[0]?.id ?? null;
}

function message(reason: unknown): string {
  return reason instanceof Error ? reason.message : 'The candidate field could not be updated.';
}
