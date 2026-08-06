import { RiArrowLeftLine, RiCheckLine, RiExternalLinkLine } from '@remixicon/react';

import type { Decision } from '@/client/types.gen';
import { FieldWorkspace } from '@/components/field-workspace';
import { ResponseWorkspace } from '@/components/response-workspace';
import { DecisionPackageWorkspace } from '@/components/decision-package-workspace';
import * as Button from '@/components/ui/button';

export function PolicyDecisionView({ decision, onNewReport, onReviewEvidence }: {
  decision: Decision;
  onNewReport: () => void;
  onReviewEvidence: () => void;
}) {
  const copy = tierCopy(decision.tier);
  const canScreenFields = decision.tier === 'STANDARD' || decision.tier === 'ELEVATED';

  return (
    <section className="screening-case" aria-labelledby="policy-title">
      <div className="case-toolbar">
        <button className="back-button" type="button" onClick={onNewReport}><RiArrowLeftLine aria-hidden="true" /> New report</button>
        <button className="text-button" type="button" onClick={onReviewEvidence}>Review lab evidence</button>
      </div>

      <header className={`case-outcome case-outcome--${decision.tier.toLowerCase()}`}>
        <div className="case-identity">
          <span>Batch screening</span>
          <strong>{decision.batchIdentifier}</strong>
          <p>{decision.facilityName}</p>
        </div>
        <div className="case-result">
          <span className="case-symbol" aria-hidden="true">{copy.symbol}</span>
          <div>
            <h1 id="policy-title">{copy.title}</h1>
            <p>{decision.explanation}</p>
          </div>
        </div>
        <dl className="case-values">
          {decision.analytes?.map((analyte) => (
            <div key={analyte.canonicalAnalyte}>
              <dt>{analyte.canonicalAnalyte}</dt>
              <dd>{analyte.isNonDetect ? `< ${analyte.upperBoundUgKgDry ?? 'unknown'}` : analyte.normalizedValueUgKgDry ?? 'unknown'} <span>µg/kg</span></dd>
            </div>
          ))}
        </dl>
        {decision.maximumApplicationRateDryTonsPerAcre ? <p className="case-effect"><span>PFAS rate ceiling</span><strong>{decision.maximumApplicationRateDryTonsPerAcre} dry tons/acre</strong></p> : null}
        {decision.prohibitedActions?.includes('LAND_APPLICATION') ? <p className="case-effect"><span>Blocked action</span><strong>Land application</strong></p> : null}
        {decision.blockingReason ? <p className="case-blocker">{decision.blockingReason}</p> : null}
        <p className="case-boundary">This is the batch status. Each field still requires its own boundary and application facts.</p>
      </header>

      <details className="policy-context">
        <summary>
          <span><strong>{decision.requirements?.length ?? 0} batch requirement{decision.requirements?.length === 1 ? '' : 's'}</strong><small>Michigan EGLE · version {decision.rulePack.version}</small></span>
          <span>View policy</span>
        </summary>
        <div className="policy-context__body">
          {decision.requirements?.length ? (
            <ol className="requirement-list">
              {decision.requirements.map((requirement, index) => (
                <li key={requirement.id}>
                  <span className="requirement-index">{String(index + 1).padStart(2, '0')}</span>
                  <div><h2>{requirement.title}</h2><p>{requirement.detail}</p></div>
                  <div className="requirement-timing"><RiCheckLine aria-hidden="true" /><span>{requirement.timing}</span></div>
                </li>
              ))}
            </ol>
          ) : <p className="empty-requirements">Resolve the lab evidence before requirements can be determined.</p>}
          <div className="policy-source">
            <div>
              <span>Policy source</span>
              <strong>Michigan EGLE interim strategy</strong>
              <p>Effective {formatDate(decision.rulePack.effectiveFrom)} · retrieved {formatDate(decision.rulePack.retrievedAt)}</p>
            </div>
            <Button.Root asChild variant="neutral" mode="stroke" size="small">
              <a href={decision.rulePack.sourceUrl} target="_blank" rel="noreferrer">Open source <RiExternalLinkLine aria-hidden="true" /></a>
            </Button.Root>
          </div>
          <details className="policy-technical">
            <summary>Decision record</summary>
            <dl>
              <div><dt>Matched rule</dt><dd>{decision.matchedRuleId ?? 'No rule matched'}</dd></div>
              <div><dt>Lab evidence version</dt><dd>{decision.reportVersion}</dd></div>
              <div><dt>Rule-pack checksum</dt><dd>{decision.rulePack.checksum}</dd></div>
              <div><dt>Decision input hash</dt><dd>{decision.inputHash}</dd></div>
            </dl>
          </details>
        </div>
      </details>

      {decision.tier === 'ELEVATED' || decision.tier === 'PROHIBITED' ? <ResponseWorkspace key={`response-${decision.id}`} decision={decision} /> : null}
      {canScreenFields ? <FieldWorkspace key={decision.id} decision={decision} /> : null}
      <DecisionPackageWorkspace key={`package-${decision.id}`} decision={decision} />
    </section>
  );
}

function tierCopy(tier: string) {
  switch (tier) {
    case 'STANDARD': return { title: 'Below PFAS action thresholds', symbol: '↓' };
    case 'ELEVATED': return { title: 'Restricted land application', symbol: '!' };
    case 'PROHIBITED': return { title: 'Land application prohibited', symbol: '×' };
    default: return { title: 'Classification needs review', symbol: '?' };
  }
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value));
}
