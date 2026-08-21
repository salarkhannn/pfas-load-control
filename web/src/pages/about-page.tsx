import { RiArrowRightLine } from '@remixicon/react';

import { TopNav } from '@/components/top-nav';
import * as Button from '@/components/ui/button';

const SOURCES = {
  epa: 'https://www.epa.gov/biosolids/basic-information-about-sewage-sludge-and-biosolids',
  michiganAnnual: 'https://www.michigan.gov/mdard/-/media/Project/Websites/mdard/documents/environment/rtf/biosolids/biosolids-annual-report-2022.pdf?hash=835344DE72A521B81A5CB4D9D518503C&rev=115337db13924d00b2264561922037bf',
  michiganPfas: 'https://www.michigan.gov/egle/about/Organization/Water-Resources/biosolids/pfas-related',
  michiganPermit: 'https://www.michigan.gov/-/media/Project/Websites/egle/Documents/Programs/WRD/NPDES/General-Permits/MIG960000-biosolids-application-general-permit-2025.pdf?rev=b6f483dc8c624a1ebf78c3fff928bd1b',
  michiganLandApplication: 'https://www.michigan.gov/egle/about/organization/water-resources/biosolids/land-application',
};

const publicFacts = [
  {
    value: '4 million dry metric tons',
    fact: 'Approximately this much sewage sludge was reported to EPA in 2024; 59.5% was land applied. EPA says the dataset omits some states and smaller facilities, so it is not a complete national total.',
    source: 'U.S. EPA, 2024 reporting data',
    href: SOURCES.epa,
    note: '1',
  },
  {
    value: '90,239 dry tons',
    fact: 'Michigan reported this amount applied on farmland in 2022, with an estimated fertilizer value of $15.5 million. This is historical program data—not a FieldProof savings claim.',
    source: 'Michigan MDARD, 2022 annual report',
    href: SOURCES.michiganAnnual,
    note: '2',
  },
  {
    value: '173 facilities',
    fact: 'Submitted Michigan biosolids PFAS results in 2024. EGLE reports 89% below 20 ppb for PFOS and PFOA and 11% between 20 and 100 ppb.',
    source: 'Michigan EGLE, 2024 PFAS results',
    href: SOURCES.michiganPfas,
    note: '3',
  },
  {
    value: '6% surface · 12% injection',
    fact: 'Michigan’s 2025 general permit uses these slope limits unless EGLE approves otherwise; qualifying exceptional-quality biosolids are exempt.',
    source: 'Michigan EGLE, General Permit MIG960000',
    href: SOURCES.michiganPermit,
    note: '4',
  },
];

const decisionInputs = [
  ['Batch', 'Laboratory results and the policy version that governs them.'],
  ['Field', 'Boundary, slope, drainage, soil information, and usable acreage.'],
  ['Farm plan', 'Crop nutrient need, application method, and prior loading.'],
  ['People', 'Landowner agreement, operator records, review, and authorization.'],
];

export function AboutPage() {
  return (
    <div className="app-shell">
      <TopNav />
      <main className="page-content about-page">
        <header className="about-intro">
          <h1>Why FieldProof exists</h1>
          <p>Biosolids are nutrient-rich solids left after domestic sewage is treated. They can be recycled on farmland as fertilizer or soil conditioner, but only when the material, field, and operating plan fit together.</p>
          <p>FieldProof helps land-application contractors and utilities assemble that evidence before they propose where a batch should go. It does not authorize the application.</p>
        </header>

        <nav className="about-index" aria-label="About this project">
          <a href="#problem">The problem</a>
          <a href="#why">Why it matters</a>
          <a href="#approach">How it works</a>
          <a href="#sources">Sources</a>
        </nav>

        <section className="about-chapter" id="problem" aria-labelledby="problem-title">
          <p className="about-chapter__label">The problem</p>
          <div className="about-chapter__body">
            <h2 id="problem-title">One placement decision depends on facts kept in different places.</h2>
            <p>A field may have enough recorded acreage and still be unsuitable for the proposed method. A lab result may change the allowed rate. A missing soil test, agreement, or loading record may stop the review. Today those facts can live across reports, GIS tools, spreadsheets, email, and paper files.</p>
            <p>The risk is not only a bad calculation. It is a confident-looking answer built from incomplete evidence.</p>
            <dl className="about-inputs">
              {decisionInputs.map(([term, detail]) => <div key={term}><dt>{term}</dt><dd>{detail}</dd></div>)}
            </dl>
          </div>
        </section>

        <section className="about-chapter" id="why" aria-labelledby="why-title">
          <p className="about-chapter__label">Why it matters</p>
          <div className="about-chapter__body">
            <h2 id="why-title">Land application has real value, and the rules are field-specific.</h2>
            <p>Recycling biosolids can return nutrients and organic matter to soil. The scale is large enough to matter, while Michigan’s rules show why a batch-level answer is not enough: chemistry, slope, method, and agronomic rate all affect what can happen on a particular field.</p>
            <div className="about-facts">
              {publicFacts.map((item) => (
                <article key={item.value}>
                  <h3>{item.value}</h3>
                  <p>{item.fact}</p>
                  <a href={item.href} target="_blank" rel="noreferrer">[{item.note}] {item.source}</a>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section className="about-chapter" id="approach" aria-labelledby="approach-title">
          <p className="about-chapter__label">How it works</p>
          <div className="about-chapter__body">
            <h2 id="approach-title">The agent produces a bounded proposal, not a permit.</h2>
            <p>FieldProof reads the original batch report, applies a versioned policy, checks candidate fields, calculates only from supported inputs, and freezes the evidence used. When a critical fact is missing, it holds material and asks for the record needed to continue.</p>

            <figure className="about-example">
              <figcaption><strong>Prepared example</strong><span>Seeded demonstration data—not a customer record</span></figcaption>
              <ol>
                <li><span>1</span><p><strong>A 52-dry-ton batch needs placement.</strong> Two candidate fields have recorded operating capacity.</p></li>
                <li><span>2</span><p><strong>Mireye returns a high sampled slope on Field A.</strong> The sample converts to 16.6% grade, above Michigan’s 6% surface-application threshold.<a href={SOURCES.michiganPermit} target="_blank" rel="noreferrer">[4]</a></p></li>
                <li><span>3</span><p><strong>The engine does not allocate to the unresolved field.</strong> Field B receives 28 dry tons; 24 dry tons remain held.</p></li>
                <li><span>4</span><p><strong>Reviewed evidence can trigger a new calculation.</strong> The agent verifies the evidence lineage, reruns the screen, and freezes a second package. A professional still controls authorization.</p></li>
              </ol>
            </figure>

            <div className="about-boundary">
              <h3>What FieldProof does</h3>
              <p>Reads, retrieves, compares, calculates, holds, requests evidence, and prepares a cited handoff.</p>
              <h3>What it does not do</h3>
              <p>It does not prove whole-field terrain suitability from finite samples, approve a field, accept liability, or schedule an application.</p>
            </div>

            <Button.Root asChild variant="primary" mode="filled" size="small"><a href="/judge-demo">Open the prepared case <RiArrowRightLine aria-hidden="true" /></a></Button.Root>
          </div>
        </section>

        <section className="about-chapter about-source-list" id="sources" aria-labelledby="sources-title">
          <p className="about-chapter__label">Sources</p>
          <div className="about-chapter__body">
            <h2 id="sources-title">Public data used on this page</h2>
            <p>Dates and limitations are kept beside the claims above. These sources do not validate FieldProof’s pricing, labor savings, or customer demand.</p>
            <ol>
              <li><a href={SOURCES.epa} target="_blank" rel="noreferrer">U.S. EPA — Basic information about sewage sludge and biosolids</a><span>2024 reported national generation and management practices; EPA states the dataset is incomplete.</span></li>
              <li><a href={SOURCES.michiganAnnual} target="_blank" rel="noreferrer">Michigan MDARD — Biosolids Annual Report 2022</a><span>Michigan farmland tonnage and estimated fertilizer value for 2022.</span></li>
              <li><a href={SOURCES.michiganPfas} target="_blank" rel="noreferrer">Michigan EGLE — PFAS in biosolids</a><span>2024 facility results, thresholds, monitoring frequency, and farmland context.</span></li>
              <li><a href={SOURCES.michiganPermit} target="_blank" rel="noreferrer">Michigan EGLE — General Permit MIG960000 (2025)</a><span>Land-application conditions including slope, agronomic rate, consent, and notification.</span></li>
              <li><a href={SOURCES.michiganLandApplication} target="_blank" rel="noreferrer">Michigan EGLE — Land application information</a><span>Plain-language definition, soil benefits, and the role of crop nutrient recommendations.</span></li>
            </ol>
          </div>
        </section>
      </main>
    </div>
  );
}
