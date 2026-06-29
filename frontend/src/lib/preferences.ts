export type Preferences = {
  favoriteTeams: string[];
  esportsLeagues: string[];
};

export const MAX_FAVORITE_TEAMS = 10;

// Sensible default leagues shown on the eSports page until the user customises them.
export const DEFAULT_LEAGUES = ['LCK', 'LEC', 'LCS', 'LPL', 'MSI', 'Worlds'];

export function defaultPreferences(): Preferences {
  return { favoriteTeams: [], esportsLeagues: [...DEFAULT_LEAGUES] };
}

// Fuzzy league matching mirrors Antigrav: exact code match plus keyword
// substrings so e.g. selecting "MSI" also matches "Mid-Season Invitational".
const LEAGUE_KEYWORDS: Record<string, string[]> = {
  LCK: ['lck'],
  LEC: ['lec'],
  LCS: ['lcs', 'lta', 'championship series'],
  LPL: ['lpl'],
  MSI: ['msi', 'mid-season', 'mid season'],
  Worlds: ['worlds', 'world championship'],
  International: ['msi', 'worlds', 'first stand', 'mid-season'],
  'Prime League': ['prime league'],
  'EMEA Masters': ['emea masters']
};

export function matchesLeague(leagueName: string, toggleId: string): boolean {
  if (!leagueName) return false;
  const ln = leagueName.toLowerCase();
  if (ln === toggleId.toLowerCase()) return true;
  const keywords = LEAGUE_KEYWORDS[toggleId];
  if (keywords) return keywords.some((kw) => ln.includes(kw));
  return false;
}
