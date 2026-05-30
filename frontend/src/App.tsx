import { useEffect, useState, useCallback } from 'react';
import { fetchHosts } from './api/client';
import { Dashboard } from './pages/Dashboard';
import { ConnectorsPage } from './pages/ConnectorsPage';
import { AddHostWizard } from './components/AddHostWizard';
import type { HostStatus } from './types';
import './App.css';

type View = 'dashboard' | 'connectors';

export default function App() {
  const [view, setView] = useState<View>('dashboard');
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
        <div className="app-header__brand">
          <h1>Updara</h1>
          <span className="app-header__tagline">Update Radar for your entire stack.</span>
        </div>
        <nav className="app-nav">
          <button
            className={`nav-tab ${view === 'dashboard' ? 'active' : ''}`}
            onClick={() => setView('dashboard')}
          >
            Dashboard
          </button>
          <button
            className={`nav-tab ${view === 'connectors' ? 'active' : ''}`}
            onClick={() => setView('connectors')}
          >
            Connectors
          </button>
        </nav>
        <div className="app-header__actions">
          {view === 'dashboard' && (
            <>
              {updateCount > 0 && (
                <span className="badge badge--warning">
                  {updateCount} Update{updateCount > 1 ? 's' : ''}
                </span>
              )}
              <span className="app-header__hosts">
                {hosts.length} Host{hosts.length !== 1 ? 's' : ''}
              </span>
              <button className="btn-secondary" onClick={load} disabled={loading}>
                {loading ? 'Aktualisiere…' : 'Aktualisieren'}
              </button>
              <button className="btn-primary" onClick={() => setShowWizard(true)}>
                + Host hinzufügen
              </button>
            </>
          )}
        </div>
      </header>
      <main>
        {view === 'dashboard' && (
          <Dashboard hosts={hosts} loading={loading} error={error} />
        )}
        {view === 'connectors' && <ConnectorsPage />}
      </main>
      {showWizard && (
        <AddHostWizard onClose={() => { setShowWizard(false); load(); }} />
      )}
    </div>
  );
}
