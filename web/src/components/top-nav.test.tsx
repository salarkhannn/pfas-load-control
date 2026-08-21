import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { TopNav } from '@/components/top-nav';

afterEach(cleanup);

describe('TopNav', () => {
  it.each([
    ['/', 'New case'],
    ['/judge-demo', 'Prepared case'],
    ['/coordination', 'Coordination'],
    ['/coordination/workflow/workflow-1', 'Coordination'],
    ['/about', 'About'],
  ])('marks the actual workspace for %s', (path, currentLabel) => {
    window.history.replaceState(null, '', path);
    render(<TopNav />);

    const primary = within(screen.getByRole('navigation', { name: 'Primary' }));
    const links = [
      primary.getByRole('link', { name: 'New case' }),
      primary.getByRole('link', { name: 'Prepared case' }),
      primary.getByRole('link', { name: 'Coordination' }),
      primary.getByRole('link', { name: 'About' }),
    ];

    expect(links.map((link) => link.getAttribute('href'))).toEqual(['/', '/judge-demo', '/coordination', '/about']);
    expect(new Set(links.map((link) => link.getAttribute('href'))).size).toBe(4);
    expect(primary.getByRole('link', { name: currentLabel })).toHaveAttribute('aria-current', 'page');
  });

  it('keeps data access in setup and marks it as current', () => {
    window.history.replaceState(null, '', '/data-access');
    render(<TopNav />);

    expect(screen.getByRole('link', { name: 'Data access' })).toHaveAttribute('href', '/data-access');
    expect(screen.getByRole('link', { name: 'Data access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByText('Setup', { selector: 'summary' }).parentElement).toHaveClass('utility-nav--active');
  });
});
