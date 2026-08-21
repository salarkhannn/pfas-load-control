export function TopNav() {
  const path = window.location.pathname.replace(/\/+$/, '') || '/';

  const navItems = [
    { href: '/', label: 'New case', active: path === '/' },
    { href: '/judge-demo', label: 'Prepared case', active: path === '/judge-demo' },
    { href: '/coordination', label: 'Coordination', active: path.startsWith('/coordination') },
  ];
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
          <a key={item.href} className={`topnav__link${item.active ? ' topnav__link--active' : ''}`} href={item.href} aria-current={item.active ? 'page' : undefined}>
            {item.label}
          </a>
        ))}
      </nav>
      <details className="mobile-stage-nav">
        <summary>Menu</summary>
        <div>{navItems.map((item) => <a key={item.href} href={item.href} aria-current={item.active ? 'page' : undefined}>{item.label}</a>)}</div>
      </details>
      <details className={`utility-nav${path === '/data-access' ? ' utility-nav--active' : ''}`}>
        <summary>Setup</summary>
        <div>
          <a href="/data-access" aria-current={path === '/data-access' ? 'page' : undefined}>Data access</a>
        </div>
      </details>
    </header>
  );
}
