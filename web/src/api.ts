import { client } from '@/client/client.gen';
import {
  classifyLabReport,
  approveAction,
  confirmFieldBoundary,
  confirmFieldParcel,
  confirmLabReport,
  confirmResponseFacilityLocation,
  correctLabReport,
  createCandidateField,
  createDecisionPackage,
  createPlacementEvaluation,
  createReadinessRun,
  executeAction,
  createPfasResponse,
  getFieldContext,
  getLatestDecisionPackage,
  getLabIntakeContext,
  getLabReport,
  getLatestReadinessRun,
  getLatestPhysicalEvaluation,
  getLatestPlacementEvaluation,
  getLatestPfasResponse,
  getLatestResponseFacilityLocation,
  getActionCenter,
  getPhysicalEvaluation,
  getPolicyClassification,
  getPfasResponse,
  getReadinessRun,
  importCandidateFields,
  prepareActionCenter,
  rejectAction,
  resolveCandidateField,
  resolveResponseFacilityLocation,
  selectFieldLocation,
  setFieldGeometry,
  startPhysicalEvaluation,
  updateFieldDetails,
  updateActionPayload,
  uploadLabReport,
} from '@/client/sdk.gen';
import type {
  Context,
  CorrectionWritable,
  CreateInputWritable,
  Decision,
  DecisionPackage,
  DetailsInputWritable,
  ErrorModel,
  Evaluation,
  Field,
  FieldContext,
  Import,
  PlacementPlan,
  PlanInputWritable,
  Report,
  Run,
  FacilityLocation,
  ResponseRun,
  Center,
  ControlledAction,
  DecisionInputWritable,
  UpdatePayloadInputWritable,
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

export async function confirmUploadedBoundary(workspaceKey: string, id: string): Promise<Field> {
  const result = await confirmFieldBoundary({ headers: workspaceHeaders(workspaceKey), path: { id } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The field boundary could not be confirmed.');
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

export async function loadLatestPlacementPlan(
  workspaceKey: string,
  decisionId: string,
  signal?: AbortSignal,
): Promise<PlacementPlan | null> {
  const result = await getLatestPlacementEvaluation({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The draft placement plan could not be loaded.');
}

export async function createPlacementPlan(
  workspaceKey: string,
  decisionId: string,
  body: PlanInputWritable,
): Promise<PlacementPlan> {
  const result = await createPlacementEvaluation({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, body,
  });
  if (result.data) return result.data.evaluation;
  throw apiError(result.error, 'The fields could not be compared.');
}

export async function loadLatestResponseLocation(
  workspaceKey: string,
  decisionId: string,
  signal?: AbortSignal,
): Promise<FacilityLocation | null> {
  const result = await getLatestResponseFacilityLocation({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The treatment plant location could not be loaded.');
}

export async function resolveTreatmentPlantLocation(
  workspaceKey: string,
  decisionId: string,
  body: { kind: 'address' | 'coord'; input: string },
): Promise<FacilityLocation> {
  const result = await resolveResponseFacilityLocation({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, body,
  });
  if (result.data) return result.data;
  throw apiError(result.error, 'The treatment plant location could not be found.');
}

export async function confirmTreatmentPlantLocation(
  workspaceKey: string,
  locationId: string,
): Promise<FacilityLocation> {
  const result = await confirmResponseFacilityLocation({
    headers: workspaceHeaders(workspaceKey), path: { id: locationId },
  });
  if (result.data) return result.data;
  throw apiError(result.error, 'The treatment plant location could not be confirmed.');
}

export async function loadLatestPfasResponse(
  workspaceKey: string,
  decisionId: string,
  signal?: AbortSignal,
): Promise<ResponseRun | null> {
  const result = await getLatestPfasResponse({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The required response could not be loaded.');
}

export async function loadPfasResponse(
  workspaceKey: string,
  runId: string,
  signal?: AbortSignal,
): Promise<ResponseRun> {
  const result = await getPfasResponse({
    headers: workspaceHeaders(workspaceKey), path: { id: runId }, signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  throw apiError(result.error, 'The required response could not be refreshed.');
}

export async function startPfasResponse(
  workspaceKey: string,
  decisionId: string,
  facilityLocationId: string,
): Promise<ResponseRun> {
  const result = await createPfasResponse({
    headers: workspaceHeaders(workspaceKey),
    path: { id: decisionId },
    body: { facilityLocationId },
  });
  if (result.data) return result.data.run;
  throw apiError(result.error, 'The required response could not be prepared.');
}

export async function loadLatestDecisionPackage(
  workspaceKey: string,
  decisionId: string,
  signal?: AbortSignal,
): Promise<DecisionPackage | null> {
  const result = await getLatestDecisionPackage({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId }, signal,
  });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The decision package could not be loaded.');
}

export async function generateDecisionPackage(workspaceKey: string, decisionId: string): Promise<DecisionPackage> {
  const result = await createDecisionPackage({
    headers: workspaceHeaders(workspaceKey), path: { id: decisionId },
  });
  if (result.data) return result.data.package;
  throw apiError(result.error, 'The decision package could not be generated.');
}

export async function downloadDecisionPackage(
  workspaceKey: string,
  packageId: string,
  format: 'html' | 'pdf' | 'json',
): Promise<void> {
  const response = await fetch(`${baseUrl}/api/v1/decision-packages/${packageId}/exports/${format}`, {
    headers: workspaceHeaders(workspaceKey),
  });
  if (!response.ok) throw new Error('The package export could not be downloaded.');
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? `pfas-decision-package.${format}`;
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export async function loadActionCenter(
  workspaceKey: string,
  packageId: string,
  signal?: AbortSignal,
): Promise<Center | null> {
  const result = await getActionCenter({ headers: workspaceHeaders(workspaceKey), path: { id: packageId }, signal });
  if (result.data) return result.data;
  throwIfAborted(signal);
  if (result.response?.status === 404) return null;
  throw apiError(result.error, 'The approval workspace could not be loaded.');
}

export async function createActionCenter(workspaceKey: string, packageId: string): Promise<Center> {
  const result = await prepareActionCenter({ headers: workspaceHeaders(workspaceKey), path: { id: packageId } });
  if (result.data) return result.data;
  throw apiError(result.error, 'The approval workspace could not be prepared.');
}

export async function saveActionPayload(
  workspaceKey: string,
  actionId: string,
  body: UpdatePayloadInputWritable,
): Promise<ControlledAction> {
  const result = await updateActionPayload({ headers: workspaceHeaders(workspaceKey), path: { id: actionId }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The action payload could not be saved.');
}

export async function approveExactAction(
  workspaceKey: string,
  actionId: string,
  body: DecisionInputWritable,
): Promise<ControlledAction> {
  const result = await approveAction({ headers: workspaceHeaders(workspaceKey), path: { id: actionId }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The action could not be approved.');
}

export async function rejectExactAction(
  workspaceKey: string,
  actionId: string,
  body: DecisionInputWritable,
): Promise<ControlledAction> {
  const result = await rejectAction({ headers: workspaceHeaders(workspaceKey), path: { id: actionId }, body });
  if (result.data) return result.data;
  throw apiError(result.error, 'The action could not be rejected.');
}

export async function executeApprovedAction(
  workspaceKey: string,
  actionId: string,
  idempotencyKey: string,
): Promise<ControlledAction> {
  const result = await executeAction({
    headers: { ...workspaceHeaders(workspaceKey), 'Idempotency-Key': idempotencyKey },
    path: { id: actionId },
  });
  if (result.data) return result.data;
  throw apiError(result.error, 'The approved action could not be completed.');
}

export async function downloadActionHandoffFile(
  workspaceKey: string,
  executionId: string,
): Promise<void> {
  const response = await fetch(`${baseUrl}/api/v1/execution-attempts/${executionId}/handoff`, {
    headers: workspaceHeaders(workspaceKey),
  });
  if (!response.ok) throw new Error('The operator handoff could not be downloaded.');
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? 'pfas-action-handoff.json';
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
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
