import { useCallback, useEffect, useMemo, useState } from 'react';

import {
	approveExactAction,
	createActionCenter,
	downloadActionHandoffFile,
	executeApprovedAction,
	loadActionCenter,
	rejectExactAction,
	saveActionPayload,
} from '@/api';
import type { Center, ControlledAction, DecisionInputWritable, UpdatePayloadInputWritable } from '@/client/types.gen';
import { getWorkspaceKey } from '@/utils/workspace-key';

export function useActionCenter(packageId: string) {
	const workspaceKey = useMemo(() => getWorkspaceKey(), []);
	const [value, setValue] = useState<Center | null>(null);
	const [isLoading, setIsLoading] = useState(true);
	const [busy, setBusy] = useState<string | null>(null);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		const controller = new AbortController();
		async function load() {
			const existing = await loadActionCenter(workspaceKey, packageId, controller.signal);
			if (controller.signal.aborted) return;
			setValue(existing ?? await createActionCenter(workspaceKey, packageId));
		}
		load().catch((reason: unknown) => {
			if (!controller.signal.aborted) setError(message(reason));
		}).finally(() => {
			if (!controller.signal.aborted) setIsLoading(false);
		});
		return () => controller.abort();
	}, [packageId, workspaceKey]);

	const replace = useCallback((next: ControlledAction) => {
		setValue((current) => current ? { ...current, actions: (current.actions ?? []).map((item) => item.id === next.id ? next : item) } : current);
	}, []);

	const perform = useCallback(async <T,>(key: string, work: () => Promise<T>): Promise<T> => {
		setBusy(key);
		setError(null);
		try { return await work(); }
		catch (reason) { setError(message(reason)); throw reason; }
		finally { setBusy(null); }
	}, []);

	const save = useCallback((actionId: string, payload: UpdatePayloadInputWritable) => perform(`save:${actionId}`, async () => {
		const next = await saveActionPayload(workspaceKey, actionId, payload);
		replace(next);
		return next;
	}), [perform, replace, workspaceKey]);

	const decide = useCallback((kind: 'approve' | 'reject', actionId: string, input: DecisionInputWritable) => perform(`${kind}:${actionId}`, async () => {
		const next = kind === 'approve'
			? await approveExactAction(workspaceKey, actionId, input)
			: await rejectExactAction(workspaceKey, actionId, input);
		replace(next);
		return next;
	}), [perform, replace, workspaceKey]);

	const execute = useCallback((actionId: string) => perform(`execute:${actionId}`, async () => {
		const next = await executeApprovedAction(workspaceKey, actionId, crypto.randomUUID());
		replace(next);
		return next;
	}), [perform, replace, workspaceKey]);

	const download = useCallback((executionId: string) => perform(`download:${executionId}`, () => downloadActionHandoffFile(workspaceKey, executionId)), [perform, workspaceKey]);

	return { value, isLoading, busy, error, save, decide, execute, download };
}

function message(reason: unknown): string {
	return reason instanceof Error ? reason.message : 'The action could not be completed.';
}
