import { useCallback, useEffect, useMemo, useState } from 'react';

import type { Context, CorrectionWritable, Decision, Report } from '@/client/types.gen';
import {
  NotFoundError,
  createPolicyDecision,
  confirmLabEvidence,
  loadLabContext,
  loadLabReport,
  loadPolicyDecision,
  saveLabEvidence,
  submitLabReport,
} from '@/api';
import { getCurrentReportId, getWorkspaceKey, setCurrentReportId } from '@/utils/workspace-key';

const ACTIVE_STATUSES = new Set(['UPLOADED', 'PROCESSING']);

export function useLabReport() {
  const workspaceKey = useMemo(() => getWorkspaceKey(), []);
  const [context, setContext] = useState<Context | null>(null);
  const [report, setReport] = useState<Report | null>(null);
  const [decision, setDecision] = useState<Decision | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isClassifying, setIsClassifying] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    const reportId = getCurrentReportId();
    Promise.all([
      loadLabContext(workspaceKey, controller.signal),
      reportId ? loadLabReport(workspaceKey, reportId, controller.signal) : Promise.resolve(null),
    ]).then(([nextContext, nextReport]) => {
      if (!active) return;
      setContext(nextContext);
      setReport(nextReport);
      setError(null);
    }).catch((reason: Error) => {
      if (!active || reason.name === 'AbortError') return;
      if (reportId && (reason as NotFoundError).notFound) {
        setCurrentReportId(null);
        setReport(null);
        setError(null);
        return;
      }
      setError(reason.message);
    }).finally(() => {
      if (active) setIsLoading(false);
    });
    return () => {
      active = false;
      controller.abort();
    };
  }, [workspaceKey]);

  useEffect(() => {
    const reportId = report?.id;
    const reportStatus = report?.status;
    if (!reportId || !reportStatus || !ACTIVE_STATUSES.has(reportStatus)) return;
    const controller = new AbortController();
    let failures = 0;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const nextReport = await loadLabReport(workspaceKey, reportId, controller.signal);
        failures = 0;
        setReport(nextReport);
        setError(null);
      } catch (reason: unknown) {
        if ((reason as Error).name === 'AbortError') return;
        failures += 1;
        setError((reason as Error).message);
      }
      timer = window.setTimeout(poll, Math.min(1_500 * 2 ** failures, 15_000));
    };
    void poll();
    return () => {
      controller.abort();
      if (timer) window.clearTimeout(timer);
    };
  }, [report?.id, report?.status, workspaceKey]);

  useEffect(() => {
    if (!report || report.status !== 'CONFIRMED' || decision || isClassifying) return;
    const controller = new AbortController();
    let active = true;
    loadPolicyDecision(workspaceKey, report.id, controller.signal)
      .then((existing) => existing ?? createPolicyDecision(workspaceKey, report.id, controller.signal))
      .then((next) => {
        if (active) {
          setDecision(next);
          setError(null);
        }
      })
      .catch((reason: Error) => {
        if (active && reason.name !== 'AbortError') setError(reason.message);
      })
      .finally(() => {
        if (active) setIsClassifying(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [decision, isClassifying, report, workspaceKey]);

  const upload = useCallback(async (input: Parameters<typeof submitLabReport>[1]) => {
    setIsSubmitting(true);
    setError(null);
    try {
      const next = await submitLabReport(workspaceKey, input);
      setCurrentReportId(next.id);
      setReport(next);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setIsSubmitting(false);
    }
  }, [workspaceKey]);

  const confirm = useCallback(async (correction: CorrectionWritable, changed: boolean) => {
    if (!report) return;
    setIsSubmitting(true);
    setError(null);
    try {
      let next = report;
      if (changed || (report.gaps?.length ?? 0) > 0) {
        next = await saveLabEvidence(workspaceKey, report.id, correction);
      }
      if (next.status === 'READY_TO_CONFIRM') {
        next = await confirmLabEvidence(workspaceKey, report.id);
      }
      setReport(next);
      if (next.status === 'CONFIRMED') {
        setIsClassifying(true);
        const policyDecision = await createPolicyDecision(workspaceKey, report.id);
        setDecision(policyDecision);
      }
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setIsSubmitting(false);
      setIsClassifying(false);
    }
  }, [report, workspaceKey]);

  const startNew = useCallback(() => {
    setCurrentReportId(null);
    setReport(null);
    setDecision(null);
    setError(null);
  }, []);

  const classificationPending = isClassifying || (report?.status === 'CONFIRMED' && !decision && !error);
  return { workspaceKey, context, report, decision, isLoading, isSubmitting, isClassifying: classificationPending, error, upload, confirm, startNew };
}
