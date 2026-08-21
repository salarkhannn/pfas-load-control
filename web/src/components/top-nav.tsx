import { useEffect, useState } from 'react';

export function TopNav() {
  const path = window.location.pathname;
  const [hash, setHash] = useState(() => window.location.hash);
  useEffect(() => {
    const updateHash = () => setHash(window.location.hash);
    window.addEventListener('hashchange', updateHash);
    return () => window.removeEventListener('hashchange', updateHash);
  }, []);

  const onJudgePath = path === '/judge-demo';
  const navItems = [
    { href: '/', label: 'Cases', path: '/', hash: '' },
    { href: onJudgePath ? '/judge-demo#evidence' : '/', label: 'Evidence', path: onJudgePath ? '/judge-demo' : '/', hash: onJudgePath ? '#evidence' : '' },
    { href: '/judge-demo#fields', label: 'Fields', path: '/judge-demo', hash: '#fields' },
    { href: '/judge-demo#decision-package', label: 'Decision package', path: '/judge-demo', hash: '#decision-package' },
  ];
  const current = (item: { path: string; hash: string; label: string }) => {
    if (item.label === 'Cases') return false;
    return path === item.path && (hash === item.hash || (item.label === 'Evidence' && path === '/' && hash === '#evidence'));
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
          <a key={item.href} className={`topnav__link${current(item) ? ' topnav__link--active' : ''}`} href={item.href} aria-current={current(item) ? 'location' : undefined}>
            {item.label}
          </a>
        ))}
      </nav>
      <details className="mobile-stage-nav">
        <summary>Stages</summary>
        <div>{navItems.map((item) => <a key={item.href} href={item.href} aria-current={current(item) ? 'location' : undefined}>{item.label}</a>)}</div>
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
