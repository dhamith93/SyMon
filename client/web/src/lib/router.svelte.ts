// A small history router: the current path and query as reactive state

export type Page =
  | { name: 'fleet' }
  | { name: 'host'; host: string }
  | { name: 'custom'; host: string }
  | { name: 'alerts' }
  | { name: 'notfound' };

export const location = $state({
  path: window.location.pathname,
  query: new URLSearchParams(window.location.search),
});

function sync() {
  location.path = window.location.pathname;
  location.query = new URLSearchParams(window.location.search);
}

window.addEventListener('popstate', sync);

// navigate to a path within the app. replace keeps the back button tidy
// for changes like picking a time range.
export function navigate(url: string, replace = false) {
  if (replace) {
    history.replaceState(null, '', url);
  } else {
    history.pushState(null, '', url);
    window.scrollTo(0, 0);
  }
  sync();
}

// same-origin links are handled by the router instead of reloading
export function handleLinkClick(event: MouseEvent) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const anchor = (event.target as HTMLElement).closest('a');
  if (!anchor || anchor.target || anchor.hasAttribute('download')) return;
  const url = new URL(anchor.href, window.location.href);
  if (url.origin !== window.location.origin || url.pathname.startsWith('/api/')) return;
  event.preventDefault();
  navigate(url.pathname + url.search);
}

export function match(path: string): Page {
  if (path === '/' || path === '') return { name: 'fleet' };
  if (path === '/alerts') return { name: 'alerts' };
  const host = path.match(/^\/hosts\/([^/]+)(\/custom)?\/?$/);
  if (host) {
    const name = decodeURIComponent(host[1]);
    return host[2] ? { name: 'custom', host: name } : { name: 'host', host: name };
  }
  return { name: 'notfound' };
}

export function hostPath(host: string, custom = false): string {
  return `/hosts/${encodeURIComponent(host)}${custom ? '/custom' : ''}`;
}
