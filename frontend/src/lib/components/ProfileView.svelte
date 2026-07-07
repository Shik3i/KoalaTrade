<script lang="ts">
  import { Download, KeyRound, LogIn, LogOut, Search, Star, Trash2, Trophy, UserCircle2 } from '@lucide/svelte';
  import type { EsportsTeamInfo, SessionUser } from '../api';
  import { MAX_FAVORITE_TEAMS } from '../preferences';
  import { formatMoney } from '../portfolio';

  export let favoriteTeams: string[] = [];
  export let esportsLeagues: string[] = [];
  export let teams: EsportsTeamInfo[] = [];
  export let teamsLoading = false;
  export let leagueOptions: string[] = [];
  export let clientId = '';
  export let equityCents = 0;
  export let startingCents = 0;
  export let user: SessionUser | null = null;
  export let registrationOpen = true;
  export let authBusy = false;
  export let onToggleTeam: (code: string) => void;
  export let onToggleLeague: (id: string) => void;
  export let onLogin: (username: string, password: string) => Promise<void>;
  export let onRegister: (username: string, password: string) => Promise<void>;
  export let onLogout: () => Promise<void>;
  export let onUpdateAccount: (displayName: string) => Promise<void>;
  export let onChangePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  export let onExportAccount: () => Promise<void>;
  export let onDeletePortfolioData: (password: string) => Promise<void>;
  export let onDeleteAccount: (password: string) => Promise<void>;

  let teamQuery = '';
  let authMode: 'login' | 'register' = 'login';
  let username = '';
  let password = '';
  let authError = '';
  let displayName = '';
  let currentPassword = '';
  let newPassword = '';
  let profileError = '';
  let passwordError = '';
  let dangerPassword = '';
  let dangerError = '';
  let busyAction: '' | 'profile' | 'password' | 'export' | 'portfolio' | 'account' = '';
  let loadedUserId = '';

  $: q = teamQuery.trim().toLowerCase();
  $: results = (q
    ? teams.filter((team) => `${team.code} ${team.name} ${team.league}`.toLowerCase().includes(q))
    : teams
  ).slice(0, 60);
  $: favoriteTeamInfos = favoriteTeams.map(
    (code) => teams.find((team) => team.code === code) ?? { code, name: code, league: '', image: '' }
  );
  $: atLimit = favoriteTeams.length >= MAX_FAVORITE_TEAMS;
  $: if (user?.id !== loadedUserId) {
    loadedUserId = user?.id ?? '';
    displayName = user?.displayName ?? '';
  }

  async function submitAuth() {
    authError = '';
    try {
      if (authMode === 'register') {
        await onRegister(username, password);
      } else {
        await onLogin(username, password);
      }
      password = '';
    } catch (error) {
      authError = error instanceof Error ? error.message : 'Anmeldung fehlgeschlagen';
    }
  }

  async function saveProfile() {
    profileError = '';
    busyAction = 'profile';
    try {
      await onUpdateAccount(displayName);
    } catch (error) {
      profileError = error instanceof Error ? error.message : 'Profil konnte nicht gespeichert werden';
    } finally {
      busyAction = '';
    }
  }

  async function savePassword() {
    passwordError = '';
    busyAction = 'password';
    try {
      await onChangePassword(currentPassword, newPassword);
      currentPassword = '';
      newPassword = '';
    } catch (error) {
      passwordError = error instanceof Error ? error.message : 'Passwort konnte nicht geändert werden';
    } finally {
      busyAction = '';
    }
  }

  async function exportData() {
    dangerError = '';
    busyAction = 'export';
    try {
      await onExportAccount();
    } catch (error) {
      dangerError = error instanceof Error ? error.message : 'Export fehlgeschlagen';
    } finally {
      busyAction = '';
    }
  }

  async function deletePortfolioData() {
    dangerError = '';
    busyAction = 'portfolio';
    try {
      await onDeletePortfolioData(dangerPassword);
      dangerPassword = '';
    } catch (error) {
      dangerError = error instanceof Error ? error.message : 'Portfolio-Daten konnten nicht gelöscht werden';
    } finally {
      busyAction = '';
    }
  }

  async function deleteAccount() {
    dangerError = '';
    busyAction = 'account';
    try {
      await onDeleteAccount(dangerPassword);
      dangerPassword = '';
    } catch (error) {
      dangerError = error instanceof Error ? error.message : 'Account konnte nicht gelöscht werden';
    } finally {
      busyAction = '';
    }
  }
</script>

<div class="profile">
  <section class="panel account">
    <div class="panel-head"><div><p class="eyebrow">Profil</p><h2>Dein Account</h2></div><UserCircle2 size={18} /></div>
    <div class="account-grid">
      <div><span>Status</span><strong>{user ? user.displayName : 'Lokal'}</strong></div>
      <div><span>Equity</span><strong>{formatMoney(equityCents)}</strong></div>
      <div><span>Startkapital</span><strong>{formatMoney(startingCents)}</strong></div>
      <div><span>Favoriten</span><strong>{favoriteTeams.length}/{MAX_FAVORITE_TEAMS}</strong></div>
      <div><span>Client-ID</span><strong class="mono">{clientId ? clientId.slice(0, 8) : '—'}</strong></div>
    </div>

    {#if user}
      <div class="account-row">
        <span>{user.username} · {user.role}</span>
        <button class="ghost-btn" type="button" title="Melde dich von deinem Account ab. Dein Portfolio wird lokal gespeichert." disabled={authBusy} on:click={onLogout}><LogOut size={15} /> Logout</button>
      </div>
      <div class="account-management">
        <form class="mini-form" on:submit|preventDefault={saveProfile}>
          <div class="panel-head slim"><div><p class="eyebrow">Account</p><h3>Profil</h3></div><UserCircle2 size={16} /></div>
          <label class="field" title="Anzeigename für dein Profil"><span>Display Name</span><input bind:value={displayName} type="text" autocomplete="name" title="Gib deinen gewünschten Anzeigenamen ein (mindestens 2 Zeichen)" /></label>
          {#if profileError}<p class="form-error">{profileError}</p>{/if}
          <button class="primary-button" type="submit" title="Übernehme deinen neuen Anzeigenamen" disabled={busyAction === 'profile' || displayName.trim().length < 2}>
            {busyAction === 'profile' ? 'Speichere …' : 'Profil speichern'}
          </button>
        </form>

        <form class="mini-form" on:submit|preventDefault={savePassword}>
          <div class="panel-head slim"><div><p class="eyebrow">Security</p><h3>Passwort</h3></div><KeyRound size={16} /></div>
          <label class="field" title="Gib dein aktuelles Passwort ein"><span>Aktuelles Passwort</span><input bind:value={currentPassword} type="password" autocomplete="current-password" title="Dein aktuelles Passwort" /></label>
          <label class="field" title="Wähle ein neues sicheres Passwort"><span>Neues Passwort</span><input bind:value={newPassword} type="password" autocomplete="new-password" title="Neues Passwort (mindestens 10 Zeichen)" /></label>
          {#if passwordError}<p class="form-error">{passwordError}</p>{/if}
          <button class="primary-button" type="submit" title="Ändere dein Passwort. Mindestens 10 Zeichen erforderlich." disabled={busyAction === 'password' || currentPassword.length < 10 || newPassword.length < 10}>
            {busyAction === 'password' ? 'Ändere …' : 'Passwort ändern'}
          </button>
        </form>

        <section class="mini-form danger-zone">
          <div class="panel-head slim"><div><p class="eyebrow">Daten</p><h3>Export & Delete</h3></div><Trash2 size={16} /></div>
          <div class="danger-actions">
            <button class="ghost-btn" type="button" title="Lade alle deine Profildaten, Transaktionen und Portfoliowerte als JSON-Datei herunter (GDPR/DSGVO-konform)." disabled={busyAction === 'export'} on:click={exportData}><Download size={15} /> Export</button>
          </div>
          <label class="field" title="Wird zur Bestätigung für Daten- und Kontolöschungen benötigt."><span>Passwort für Löschaktionen</span><input bind:value={dangerPassword} type="password" autocomplete="current-password" title="Passwort zur Bestätigung" /></label>
          {#if dangerError}<p class="form-error">{dangerError}</p>{/if}
          <div class="danger-actions">
            <button class="ghost-btn danger" type="button" title="Löscht all deine Trades und Transaktionsdaten dauerhaft vom Server" disabled={busyAction === 'portfolio' || dangerPassword.length < 10} on:click={deletePortfolioData}>Portfolio-Daten löschen</button>
            <button class="ghost-btn danger" type="button" title="Löscht dein Benutzerkonto sowie alle zugehörigen Daten endgültig vom Server" disabled={busyAction === 'account' || dangerPassword.length < 10} on:click={deleteAccount}>Account löschen</button>
          </div>
        </section>
      </div>
    {:else}
      <form class="auth-form" on:submit|preventDefault={submitAuth}>
        <div class="segmented compact-segment">
          <button class:active={authMode === 'login'} type="button" title="Melde dich mit deinem bestehenden Account an" on:click={() => (authMode = 'login')}>Login</button>
          <button class:active={authMode === 'register'} type="button" title="Erstelle ein neues Benutzerkonto, falls die Registrierung geöffnet ist" disabled={!registrationOpen} on:click={() => (authMode = 'register')}>Registrieren</button>
        </div>
        <label class="field" title="Benutzername für dein Konto (mindestens 3 Zeichen)"><span>Benutzername</span><input bind:value={username} type="text" autocomplete="username" title="Benutzername eingeben" /></label>
        <label class="field" title="Passwort für dein Konto (mindestens 10 Zeichen)"><span>Passwort</span><input bind:value={password} type="password" autocomplete={authMode === 'login' ? 'current-password' : 'new-password'} title="Passwort eingeben" /></label>
        {#if authError}<p class="form-error">{authError}</p>{/if}
        <button class="primary-button" type="submit" title={authMode === 'register' ? 'Erstelle dein neues Benutzerkonto und melde dich an' : 'Melde dich an, um dein Portfolio zu synchronisieren'} disabled={authBusy || username.trim().length < 3 || password.length < 10}>
          <LogIn size={15} /> {authMode === 'register' ? 'Account erstellen' : 'Einloggen'}
        </button>
      </form>
    {/if}
  </section>

  <section class="panel">
    <div class="panel-head"><div><p class="eyebrow">eSports</p><h2>Standard-Ligen</h2></div><Trophy size={18} /></div>
    <p class="hint">Diese Ligen werden auf der eSports-Seite standardmäßig angezeigt.</p>
    <div class="league-chips">
      {#each leagueOptions as league}
        <button class:active={esportsLeagues.includes(league)} type="button" title={`Standardmäßig Spiele der Liga ${league} auf der eSports-Seite anzeigen`} on:click={() => onToggleLeague(league)}>
          {league}
        </button>
      {/each}
    </div>
  </section>

  <section class="panel">
    <div class="panel-head"><div><p class="eyebrow">eSports</p><h2>Lieblingsteams</h2></div><Star size={18} /></div>

    {#if favoriteTeamInfos.length > 0}
      <div class="fav-chips">
        {#each favoriteTeamInfos as team}
          <button class="fav-chip" type="button" on:click={() => onToggleTeam(team.code)} title={`Entferne ${team.code} (${team.name}) aus deinen Favoriten`}>
            {#if team.image}<img src={team.image} alt="" width="18" height="18" />{/if}
            <span>{team.code}</span>
            <em>×</em>
          </button>
        {/each}
      </div>
    {/if}

    <label class="search compact">
      <Search size={16} />
      <input bind:value={teamQuery} type="search" placeholder="Team suchen (z.B. T1, G2, Fnatic)" />
    </label>

    {#if teamsLoading}
      <div class="team-results">{#each Array(6) as _}<div class="skeleton-line"></div>{/each}</div>
    {:else if results.length === 0}
      <p class="empty-state">Keine Teams gefunden.</p>
    {:else}
      <div class="team-results">
        {#each results as team (team.code)}
          {@const selected = favoriteTeams.includes(team.code)}
          <div class="team-result">
            <div class="t-id">
              {#if team.image}<img src={team.image} alt="" width="26" height="26" loading="lazy" />{:else}<span class="t-fallback">{team.code}</span>{/if}
              <div><strong>{team.code}</strong><small>{team.name}{team.league ? ` · ${team.league}` : ''}</small></div>
            </div>
            <button
              class="t-action"
              class:selected
              type="button"
              title={selected ? `Entferne ${team.code} aus deinen Favoriten` : atLimit ? `Maximale Anzahl an Favoriten (${MAX_FAVORITE_TEAMS}) erreicht.` : `Füge ${team.code} zu deinen Favoriten hinzu`}
              disabled={!selected && atLimit}
              on:click={() => onToggleTeam(team.code)}
            >
              {selected ? 'Entfernen' : atLimit ? 'Limit' : '+ Favorit'}
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </section>
</div>

<style>
  .profile {
    display: grid;
    gap: 0.75rem;
    align-content: start;
    max-width: 56rem;
    margin: 0 auto;
    width: 100%;
  }

  .hint {
    margin: 0 0 0.75rem;
    color: var(--muted);
    font-size: 0.85rem;
  }

  .account-grid {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 0.6rem;
  }

  .account-grid div {
    display: grid;
    gap: 0.15rem;
    padding: 0.7rem 0.8rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .account-grid span {
    color: var(--muted);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .account-grid strong {
    font-size: 1.1rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.95rem;
  }

  .account-row,
  .account-management,
  .auth-form,
  .mini-form,
  .field {
    display: grid;
    gap: 0.55rem;
  }

  .account-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 0.8rem;
    color: var(--muted);
    font-size: 0.86rem;
  }

  .auth-form {
    max-width: 26rem;
    margin-top: 0.9rem;
  }

  .account-management {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.75rem;
    margin-top: 0.9rem;
  }

  .mini-form {
    align-content: start;
    padding: 0.8rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .panel-head.slim {
    margin-bottom: 0.1rem;
  }

  .panel-head.slim h3 {
    margin: 0;
    font-size: 1rem;
  }

  .compact-segment {
    max-width: 18rem;
  }

  .ghost-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.1rem;
    padding: 0 0.75rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.82rem;
    background: var(--panel-3);
  }

  .ghost-btn.danger {
    color: var(--red);
    border-color: var(--red-soft);
    background: var(--red-soft);
  }

  .primary-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
  }

  .danger-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }

  .league-chips,
  .fav-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .fav-chips {
    margin-bottom: 0.75rem;
  }

  .league-chips button {
    min-height: 2.1rem;
    padding: 0 0.85rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    color: var(--muted);
    font-size: 0.82rem;
    background: var(--bg-2);
    transition: 120ms ease;
  }

  .league-chips button:hover {
    border-color: var(--line-2);
    color: var(--text);
  }

  .league-chips button.active {
    color: var(--green);
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .fav-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.6rem;
    border: 1px solid var(--line-2);
    border-radius: 999px;
    color: var(--text);
    background: var(--panel-3);
  }

  .fav-chip img {
    border-radius: 4px;
  }

  .fav-chip em {
    font-style: normal;
    color: var(--muted);
  }

  .fav-chip:hover {
    border-color: var(--red);
  }

  .fav-chip:hover em {
    color: var(--red);
  }

  .team-results {
    display: grid;
    gap: 0.3rem;
    max-height: 24rem;
    margin-top: 0.6rem;
    overflow-y: auto;
    scrollbar-width: thin;
  }

  .team-result {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .t-id {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .t-id img {
    border-radius: 5px;
    object-fit: contain;
    background: rgba(255, 255, 255, 0.04);
  }

  .t-fallback {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border-radius: 5px;
    color: var(--muted);
    font-size: 0.6rem;
    background: var(--panel-3);
  }

  .t-id div {
    display: grid;
    gap: 0.05rem;
    min-width: 0;
  }

  .t-id strong {
    font-size: 0.9rem;
  }

  .t-id small {
    color: var(--muted);
    font-size: 0.72rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .t-action {
    flex-shrink: 0;
    min-height: 2rem;
    padding: 0 0.75rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.8rem;
    background: var(--panel-3);
    transition: 120ms ease;
  }

  .t-action:hover:not(:disabled) {
    border-color: var(--green);
    color: var(--green);
  }

  .t-action.selected {
    color: var(--red);
    border-color: var(--red-soft);
    background: var(--red-soft);
  }

  .t-action:disabled {
    cursor: not-allowed;
    opacity: 0.45;
  }

  .skeleton-line {
    height: 2.6rem;
    border-radius: var(--r-sm);
    background: linear-gradient(100deg, rgba(255, 255, 255, 0.03) 30%, rgba(255, 255, 255, 0.07) 50%, rgba(255, 255, 255, 0.03) 70%);
    background-size: 200% 100%;
    animation: shimmer 1.3s infinite;
  }

  @keyframes shimmer {
    to { background-position: -200% 0; }
  }

  @media (max-width: 560px) {
    .account-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .account-management {
      grid-template-columns: 1fr;
    }
  }
</style>
