import { useEffect, useState, useCallback } from 'react';
import { fetchHosts } from './api/client';
import { Dashboard } from './pages/Dashboard';
import { ConnectorsPage } from './pages/ConnectorsPage';
import { HostDetailPage } from './pages/HostDetailPage';
import { AddHostWizard } from './components/AddHostWizard';
import { useT } from './i18n';
import type { HostStatus } from './types';
import './App.css';

type View = 'dashboard' | 'connectors';

export default function App() {
  const { t, lang, setLang } = useT();
  const [view, setView] = useState<View>('dashboard');
  const [selectedHostname, setSelectedHostname] = useState<string | null>(null);
  const [hosts, setHosts] = useState<HostStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showWizard, setShowWizard] = useState(false);

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
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const updateCount = hosts.reduce(
    (n, h) => n + (h.results ?? []).filter(r => r.update_available).length,
    0,
  );

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
              <button className="btn-primary" onClick={() => setShowWizard(true)}>
                {t.app.addHost}
              </button>
            </>
          )}
          <div className="lang-toggle">
            <button className={lang === 'en' ? 'active' : ''} onClick={() => setLang('en')}>EN</button>
            <button className={lang === 'de' ? 'active' : ''} onClick={() => setLang('de')}>DE</button>
          </div>
        </div>
      </header>
      <main>
        {view === 'dashboard' && !selectedHostname && (
          <Dashboard hosts={hosts} loading={loading} error={error} onSelectHost={setSelectedHostname} />
        )}
        {view === 'dashboard' && selectedHostname && (
          <HostDetailPage hostname={selectedHostname} onBack={() => { setSelectedHostname(null); load(); }} />
        )}
        {view === 'connectors' && <ConnectorsPage />}
      </main>
      {showWizard && (
        <AddHostWizard onClose={() => { setShowWizard(false); load(); }} />
      )}
    </div>
  );
}
