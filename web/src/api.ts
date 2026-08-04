import { client } from '@/client/client.gen';
import {
  createReadinessRun,
  getLatestReadinessRun,
  getReadinessRun,
} from '@/client/sdk.gen';
import type { Run } from '@/client/types.gen';

const baseUrl = (import.meta.env.VITE_API_URL ?? 'http://localhost:8080').replace(/\/$/, '');
client.setConfig({ baseUrl });

export async function loadLatestRun(signal?: AbortSignal): Promise<Run | null> {
  const result = await getLatestReadinessRun({ signal });
  if (result.data) {
    return result.data;
  }
  if (result.response?.status === 404) {
    return null;
  }
  throw new Error('The latest data access check could not be loaded.');
}

export async function loadRun(id: string, signal?: AbortSignal): Promise<Run> {
  const result = await getReadinessRun({ path: { id }, signal });
  if (result.data) {
    return result.data;
  }
  throw new Error('The data access check could not be refreshed.');
}

export async function startRun(): Promise<Run> {
  const result = await createReadinessRun();
  if (result.data) {
    return result.data.run;
  }
  throw new Error('The data access check could not be started.');
}
