import { useEffect, useState, useCallback } from 'react';
import { fetchHosts } from './api/client';
import { Dashboard } from './pages/Dashboard';
import { ConnectorsPage } from './pages/ConnectorsPage';
import { SettingsPage } from './pages/SettingsPage';
import { StatsPage } from './pages/StatsPage';
import { HostDetailPage } from './pages/HostDetailPage';
import { LoginPage } from './pages/LoginPage';
import { AddHostWizard } from './components/AddHostWizard';
import { useT } from './i18n';
import type { HostStatus } from './types';
import './App.css';

type View = 'dashboard' | 'connectors' | 'settings' | 'stats';

export default function App() {
  const { t, lang, setLang } = useT();
  const [authed, setAuthed] = useState(() => !!localStorage.getItem('updara_token'));
  const [view, setView] = useState<View>('dashboard');
  const [selectedHostname, setSelectedHostname] = useState<string | null>(null);
  const [hosts, setHosts] = useState<HostStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [wizardInitialName, setWizardInitialName] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await fetchHosts();
      setHosts(data);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!authed) return;
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load, authed]);

  const updateCount = hosts.reduce(
    (n, h) => n + (h.results ?? []).filter(r => r.update_available).length,
    0,
  );

  if (!authed) return <LoginPage onLogin={() => setAuthed(true)} />;

  const handleLogout = () => {
    localStorage.removeItem('updara_token');
    setAuthed(false);
  };

  return (
    <div className="app">
      <header className="app-header">
        <div
          className="app-header__brand app-header__brand--clickable"
          onClick={() => { setView('dashboard'); setSelectedHostname(null); }}
        >
          <img src="/updara_logo.svg" alt="Updara" className="app-header__logo" />
          <span className="app-header__tagline">{t.app.tagline}</span>
        </div>
        <nav className="app-nav">
          <button
            className={`nav-tab ${view === 'dashboard' ? 'active' : ''}`}
            onClick={() => { setView('dashboard'); setSelectedHostname(null); }}
          >
            {t.app.nav.dashboard}
          </button>
          <button
            className={`nav-tab ${view === 'connectors' ? 'active' : ''}`}
            onClick={() => setView('connectors')}
          >
            {t.app.nav.connectors}
          </button>
          <button
            className={`nav-tab ${view === 'stats' ? 'active' : ''}`}
            onClick={() => setView('stats')}
          >
            {t.app.nav.stats}
          </button>
          <button
            className={`nav-tab ${view === 'settings' ? 'active' : ''}`}
            onClick={() => setView('settings')}
          >
            {t.app.nav.settings}
          </button>
        </nav>
        <div className="app-header__actions">
          {view === 'dashboard' && (
            <>
              {updateCount > 0 && (
                <span className="badge badge--warning">{t.app.updates(updateCount)}</span>
              )}
              <span className="app-header__hosts">{t.app.hosts(hosts.length)}</span>
              <button className="btn-secondary" onClick={load} disabled={loading}>
                {loading ? t.app.refreshing : t.app.refresh}
              </button>
              <button className="btn-primary" onClick={() => setWizardInitialName('')}>
                {t.app.addHost}
              </button>
            </>
          )}
          <button className="btn-secondary" onClick={handleLogout}>{t.login.logout}</button>
          <div className="lang-toggle">
            <button className={lang === 'en' ? 'active' : ''} onClick={() => setLang('en')}>EN</button>
            <button className={lang === 'de' ? 'active' : ''} onClick={() => setLang('de')}>DE</button>
          </div>
        </div>
      </header>
      <main>
        {view === 'dashboard' && !selectedHostname && (
          <Dashboard
            hosts={hosts}
            loading={loading}
            error={error}
            onSelectHost={setSelectedHostname}
            onAddHost={(name) => setWizardInitialName(name ?? '')}
          />
        )}
        {view === 'dashboard' && selectedHostname && (
          <HostDetailPage hostname={selectedHostname} onBack={() => { setSelectedHostname(null); load(); }} />
        )}
        {view === 'connectors' && <ConnectorsPage />}
        {view === 'stats' && <StatsPage onSelectHost={hostname => { setSelectedHostname(hostname); setView('dashboard'); }} />}
        {view === 'settings' && <SettingsPage />}
      </main>
      {wizardInitialName !== null && (
        <AddHostWizard
          initialName={wizardInitialName}
          onClose={() => { setWizardInitialName(null); load(); }}
        />
      )}
    </div>
  );
}
