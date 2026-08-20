export function TopNav() {
  const path = window.location.pathname;
  const caseHref = path === '/judge-demo' ? '/judge-demo' : '/';
  const stageHref = (anchor: string) => `${path === '/judge-demo' ? '/judge-demo' : '/'}#${anchor}`;
  const navItems = [
    { href: caseHref, label: 'Cases' },
    { href: stageHref('evidence'), label: 'Evidence' },
    { href: stageHref('fields'), label: 'Fields' },
    { href: stageHref('decision-package'), label: 'Decision package' },
  ];
  const current = (item: { href: string }) => {
    if (item.href === '/' || item.href === '/judge-demo') return path === item.href;
    if (item.href.includes('#')) return false;
    return path.startsWith(item.href);
  };
  return (
    <header className="topbar">
      <a className="brand" href="/" aria-label="FieldProof home">
        <span className="brand-mark" aria-hidden="true">
          {Array.from({ length: 9 }, (_, index) => <i key={index} />)}
        </span>
        <span>FieldProof</span>
      </a>
      <nav className="topnav" aria-label="Primary">
        {navItems.map((item) => (
          <a key={item.href} className={`topnav__link${current(item) ? ' topnav__link--active' : ''}`} href={item.href} aria-current={current(item) ? 'page' : undefined}>
            {item.label}
          </a>
        ))}
      </nav>
      <details className="mobile-stage-nav">
        <summary>Stages</summary>
        <div>{navItems.slice(1).map((item) => <a key={item.href} href={item.href}>{item.label}</a>)}</div>
      </details>
      <details className="utility-nav">
        <summary>Setup</summary>
        <div>
          <a href="/data-access">Data access</a>
          <a href="/coordination">Coordination records</a>
        </div>
      </details>
    </header>
  );
}
