import { client } from '@/client/client.gen';
import {
  classifyLabReport,
  confirmFieldParcel,
  confirmLabReport,
  correctLabReport,
  createCandidateField,
  createReadinessRun,
  getFieldContext,
  getLabIntakeContext,
  getLabReport,
  getLatestReadinessRun,
  getLatestPhysicalEvaluation,
  getPhysicalEvaluation,
  getPolicyClassification,
  getReadinessRun,
  importCandidateFields,
  resolveCandidateField,
  selectFieldLocation,
  setFieldGeometry,
  startPhysicalEvaluation,
  updateFieldDetails,
  uploadLabReport,
} from '@/client/sdk.gen';
import type {
  Context,
  CorrectionWritable,
  CreateInputWritable,
  Decision,
  DetailsInputWritable,
  ErrorModel,
  Evaluation,
  Field,
  FieldContext,
  Import,
  Report,
  Run,
} from '@/client/types.gen';

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

export async function loadLabContext(workspaceKey: string, signal?: AbortSignal): Promise<Context> {
  const result = await getLabIntakeContext({ headers: workspaceHeaders(workspaceKey), signal });
  if (result.data) return result.data;
  throwIfAborted(signal);
  throw apiError(result.error, 'The intake options could not be loaded.');
}

export async function submitLabReport(
  workspaceKey: string,
  input: { facilityName: string; batchId: string; wetMassKg: string; percentSolids: string; report: File },
): Promise<Report> {
  const wetMassKg = input.wetMassKg.trim();
  const percentSolids = input.percentSolids.trim();
  const result = await uploadLabReport({
    headers: workspaceHeaders(workspaceKey),
    body: {
      facilityName: input.facilityName,
      batchId: input.batchId,
      report: input.report,
      ...(wetMassKg ? { wetMassKg } : {}),
      ...(percentSolids ? { percentSolids } : {}),
    },
  });
  if (result.data) return result.data.report;
  throw apiError(result.error, 'The report could not be uploaded.');
}

export async function loadLabReport(workspaceKey: string, id: string, signal?: AbortSignal): Promise<Report> {
  const result = await getLabReport({ headers: workspaceHeaders(workspaceKey), path: { id }, signal });
  if (result.data) return result.data;
  throwIfAborted(signal);
  throw apiError(result.error, 'The report could not be loaded.');
}

export async function saveLabEvidence(workspaceKey: string, id: string, body: CorrectionWritable): Promise<Report> {
  const result = await correctLabReport({ headers: workspaceHeaders(workspaceKey), path: { id }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The extracted values could not be saved.');
}

export async function confirmLabEvidence(workspaceKey: string, id: string): Promise<Report> {
  const result = await confirmLabReport({ headers: workspaceHeaders(workspaceKey), path: { id } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The extracted values could not be confirmed.');
}

export async function loadPolicyDecision(workspaceKey: string, id: string, signal?: AbortSignal): Promise<Decision | null> {
  const result = await getPolicyClassification({ headers: workspaceHeaders(workspaceKey), path: { id }, signal });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The policy classification could not be loaded.');
}

export async function createPolicyDecision(workspaceKey: string, id: string, signal?: AbortSignal): Promise<Decision> {
  const result = await classifyLabReport({ headers: workspaceHeaders(workspaceKey), path: { id }, signal });
  if (result.data) return result.data.decision;
  throwIfAborted(signal);
  throw apiError(result.error, 'The policy classification could not be completed.');
}

export async function loadLabReportFile(workspaceKey: string, id: string, signal?: AbortSignal): Promise<Blob> {
  const response = await fetch(`${baseUrl}/api/v1/lab-reports/${id}/content`, {
    headers: workspaceHeaders(workspaceKey),
    signal,
  });
  if (!response.ok) throw new Error('The original report could not be loaded.');
  return response.blob();
}

export async function loadFieldContext(workspaceKey: string, facilityId?: string, signal?: AbortSignal): Promise<FieldContext> {
  const result = await getFieldContext({
    headers: workspaceHeaders(workspaceKey),
    query: facilityId ? { facilityId } : undefined,
    signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  throw apiError(result.error, 'The candidate fields could not be loaded.');
}

export async function addCandidateField(workspaceKey: string, facilityId: string, body: CreateInputWritable): Promise<Field> {
  const result = await createCandidateField({ headers: workspaceHeaders(workspaceKey), path: { facilityId }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The field could not be added.');
}

export async function resolveFieldLocation(workspaceKey: string, id: string): Promise<Field> {
  const result = await resolveCandidateField({ headers: workspaceHeaders(workspaceKey), path: { id } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The field location could not be resolved.');
}

export async function chooseFieldLocation(workspaceKey: string, id: string, candidateIndex: number): Promise<Field> {
  const result = await selectFieldLocation({
    headers: workspaceHeaders(workspaceKey), path: { id }, body: { candidateIndex },
  });
  if (result.data) return result.data;
  throw apiError(result.error, 'The location could not be selected.');
}

export async function confirmParcelBoundary(workspaceKey: string, id: string): Promise<Field> {
  const result = await confirmFieldParcel({ headers: workspaceHeaders(workspaceKey), path: { id } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The parcel boundary could not be confirmed.');
}

export async function saveFieldGeometry(workspaceKey: string, id: string, geojson: string): Promise<Field> {
  const result = await setFieldGeometry({ headers: workspaceHeaders(workspaceKey), path: { id }, body: { geojson } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The field boundary could not be saved.');
}

export async function saveFieldDetails(workspaceKey: string, id: string, body: DetailsInputWritable): Promise<Field> {
  const result = await updateFieldDetails({ headers: workspaceHeaders(workspaceKey), path: { id }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The field details could not be saved.');
}

export async function importFieldCSV(workspaceKey: string, facilityId: string, csv: File): Promise<Import> {
  const result = await importCandidateFields({
    headers: workspaceHeaders(workspaceKey), path: { facilityId }, body: { csv },
  });
  if (result.data) return result.data;
  throw apiError(result.error, 'The fields could not be imported.');
}

export async function loadLatestPhysicalEvaluation(
  workspaceKey: string,
  fieldId: string,
  signal?: AbortSignal,
): Promise<Evaluation | null> {
  const result = await getLatestPhysicalEvaluation({
    headers: workspaceHeaders(workspaceKey),
    path: { id: fieldId },
    signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The physical evidence could not be loaded.');
}

export async function loadPhysicalEvaluation(
  workspaceKey: string,
  id: string,
  signal?: AbortSignal,
): Promise<Evaluation> {
  const result = await getPhysicalEvaluation({
    headers: workspaceHeaders(workspaceKey),
    path: { id },
    signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  throw apiError(result.error, 'The physical evidence could not be refreshed.');
}

export async function startFieldPhysicalEvaluation(workspaceKey: string, fieldId: string): Promise<Evaluation> {
  const result = await startPhysicalEvaluation({
    headers: workspaceHeaders(workspaceKey),
    path: { id: fieldId },
  });
  if (result.data) return result.data.evaluation;
  throw apiError(result.error, 'The physical conditions could not be checked.');
}

function workspaceHeaders(workspaceKey: string): { 'X-Workspace-Key': string } {
  return { 'X-Workspace-Key': workspaceKey };
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw new DOMException('The request was canceled.', 'AbortError');
}

function apiError(error: unknown, fallback: string): Error {
  const detail = (error as ErrorModel | undefined)?.detail;
  return new Error(detail || fallback);
}
