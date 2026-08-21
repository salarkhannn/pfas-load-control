import { lazy, Suspense, useEffect } from 'react';
import { RiErrorWarningLine } from '@remixicon/react';

import type { DataGap } from '@/client/types.gen';
import { ReadinessChecks } from '@/components/readiness-checks';
import { RunStatus } from '@/components/run-status';
import { TopNav } from '@/components/top-nav';
import * as Alert from '@/components/ui/alert';
import * as Button from '@/components/ui/button';
import { useReadinessRun } from '@/hooks/use-readiness-run';
const LabEvidencePage = lazy(() => import('@/pages/lab-evidence-page').then((module) => ({ default: module.LabEvidencePage })));
const CoordinationPage = lazy(() => import('@/pages/coordination-page').then((module) => ({ default: module.CoordinationPage })));
const WorkflowDetailPage = lazy(() => import('@/pages/workflow-detail-page').then((module) => ({ default: module.WorkflowDetailPage })));
const JudgeDemoPage = lazy(() => import('@/pages/judge-demo-page').then((module) => ({ default: module.JudgeDemoPage })));

export function App() {
  const path = window.location.pathname.replace(/\/+$/, '') || '/';
  useEffect(() => {
    document.title = path === '/judge-demo' ? 'Judge Demo | FieldProof' : path === '/data-access' ? 'Data Access | FieldProof' : path.startsWith('/coordination') ? 'Coordination | FieldProof' : path === '/' ? 'FieldProof' : 'Page Not Found | FieldProof';
  }, [path]);
  if (path === '/data-access') return <DataAccessPage />;
  const route = path === '/'
    ? <LabEvidencePage />
    : path === '/judge-demo'
      ? <JudgeDemoPage />
      : /^\/coordination\/workflow\/[^/]+$/.test(path)
        ? <WorkflowDetailPage />
        : path === '/coordination'
          ? <CoordinationPage />
          : <NotFoundPage />;
  return <Suspense fallback={<RouteLoading />}>{route}</Suspense>;
}

function RouteLoading() {
  return <div className="app-shell"><TopNav /><main className="workspace"><div className="center-state" role="status"><span className="state-spinner" /><h1>Loading workspace</h1></div></main></div>;
}

function NotFoundPage() {
  return (
    <div className="app-shell">
      <TopNav />
      <main className="workspace">
        <section className="center-state not-found-state" aria-labelledby="not-found-title">
          <RiErrorWarningLine aria-hidden="true" />
          <h1 id="not-found-title">Page not found</h1>
          <p>This address does not match a FieldProof workspace.</p>
          <Button.Root asChild variant="primary" mode="filled" size="small"><a href="/">Start a new case</a></Button.Root>
        </section>
      </main>
    </div>
  );
}

function DataAccessPage() {
  const { run, isLoading, isStarting, error, begin } = useReadinessRun();
  const isActive = run?.status === 'QUEUED' || run?.status === 'RUNNING';
  const completedChecks = run?.steps?.filter((step) => step.status === 'SUCCEEDED').length ?? 0;
  const summary = summaryCopy(run?.status, isLoading, completedChecks);
  const actionLabel = isStarting ? 'Starting check…' : isActive ? 'Checking…' : run ? 'Check again' : 'Check access';

  return (
    <div className="app-shell">
      <TopNav />
      <main className="workspace">
        <section className="page-header" aria-labelledby="page-title">
          <div>
            <h1 id="page-title">Check data access</h1>
            <p>Confirm property data and account access before starting an investigation.</p>
          </div>
          <div className="page-action">
            <Button.Root
              className="check-button"
              variant="primary"
              mode="filled"
              size="small"
              onClick={begin}
              disabled={isLoading || isStarting || isActive}
              aria-busy={isStarting || isActive}
            >
              {actionLabel}
            </Button.Root>
            <span>Doesn’t change data or use credits</span>
          </div>
        </section>

        <div className="announcer" aria-live="polite" aria-atomic="true">
          {isActive ? 'Checking data access.' : run?.status === 'SUCCEEDED' ? 'Data access is ready.' : ''}
        </div>

        {error ? (
          <Alert.Root className="error-alert" variant="lighter" status="error" size="large" role="alert">
            <Alert.Icon as={RiErrorWarningLine} />
            <div>
              <strong>Couldn’t load data access</strong>
              <p>{error}</p>
            </div>
          </Alert.Root>
        ) : null}

        {run?.dataGaps?.map((gap) => (
          <Alert.Root className="error-alert" variant="lighter" status="error" size="large" role="alert" key={gap.id}>
            <Alert.Icon as={RiErrorWarningLine} />
            <div>
              <strong>{gapCopy(gap).title}</strong>
              <p>{gapCopy(gap).message}</p>
            </div>
          </Alert.Root>
        ))}

        <section className="check-workspace" aria-labelledby="check-summary-title">
          <div className="check-summary">
            <div className="check-summary__copy">
              <div className="check-summary__title">
                <h2 id="check-summary-title">{summary.title}</h2>
                {isLoading ? <span className="loading-state">Loading…</span> : <RunStatus status={run?.status ?? 'PENDING'} />}
              </div>
              <p>{summary.detail}</p>
            </div>
            <div className="check-summary__meta">
              {run ? <time dateTime={run.updatedAt}>Updated {relativeTime(run.updatedAt)}</time> : null}
            </div>
          </div>

          <ReadinessChecks steps={run?.steps ?? null} calls={run?.toolCalls ?? null} />
        </section>
      </main>
    </div>
  );
}

function summaryCopy(status: string | undefined, isLoading: boolean, completedChecks: number) {
  if (isLoading) {
    return { title: 'Loading data access', detail: 'Retrieving the latest check.' };
  }
  if (!status) {
    return { title: 'Not checked yet', detail: 'Check access before starting an investigation.' };
  }
  if (status === 'QUEUED' || status === 'RUNNING') {
    return { title: 'Checking data access', detail: `${completedChecks} of 3 checks complete.` };
  }
  if (status === 'SUCCEEDED') {
    return { title: 'Ready to start an investigation', detail: 'Property data, usage pricing, and account access are available.' };
  }
  if (status === 'WAITING_FOR_INPUT' || status === 'FAILED') {
    return { title: 'Data access needs attention', detail: `${completedChecks} of 3 checks passed.` };
  }
  return { title: 'Data access not checked', detail: 'Check access before starting an investigation.' };
}

function relativeTime(timestamp: string): string {
  const seconds = Math.max(0, Math.round((Date.now() - new Date(timestamp).getTime()) / 1_000));
  if (seconds < 10) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ago`;
}

function gapCopy(gap: DataGap): { title: string; message: string } {
  switch (gap.code) {
    case 'MIREYE_AUTHENTICATION_FAILED':
      return { title: 'Mireye account access failed', message: 'Connect a valid Mireye account, then check again.' };
    case 'MIREYE_ACCESS_DENIED':
      return { title: 'Required Mireye data is unavailable', message: 'Confirm that this Mireye account has access, then check again.' };
    case 'MIREYE_SCHEMA_MISMATCH':
      return { title: 'Mireye data has changed', message: 'Review the current Mireye data format before starting an investigation.' };
    case 'MIREYE_RATE_LIMITED':
      return { title: 'Mireye is temporarily limiting requests', message: 'Wait briefly, then check again.' };
    case 'MIREYE_SERVER_ERROR':
      return { title: 'Mireye is temporarily unavailable', message: 'Check again when the service is available.' };
    default:
      return { title: 'A required check needs attention', message: 'Resolve the Mireye connection, then check again.' };
  }
}
