// Light or dark: follows the OS unless the viewer picks one. The pick is
// remembered in this browser only.

export type ThemeChoice = 'system' | 'light' | 'dark';

const storageKey = 'symon-theme';
const media = window.matchMedia('(prefers-color-scheme: dark)');

function load(): ThemeChoice {
  try {
    const saved = localStorage.getItem(storageKey);
    if (saved === 'light' || saved === 'dark') return saved;
  } catch {
    // storage can be blocked, fall back to the OS setting
  }
  return 'system';
}

export const theme = $state({
  choice: load(),
  // what is on screen, charts redraw when this changes
  resolved: 'light' as 'light' | 'dark',
});

function apply() {
  theme.resolved = theme.choice === 'system' ? (media.matches ? 'dark' : 'light') : theme.choice;
  if (theme.choice === 'system') {
    delete document.documentElement.dataset.theme;
  } else {
    document.documentElement.dataset.theme = theme.choice;
  }
}

media.addEventListener('change', apply);
apply();

export function setTheme(choice: ThemeChoice) {
  theme.choice = choice;
  try {
    if (choice === 'system') localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, choice);
  } catch {
    // not remembered, still applied for this visit
  }
  apply();
}

// reads a color role from the stylesheet, for canvas drawing
export function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
