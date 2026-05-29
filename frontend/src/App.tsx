import { useEffect, useState, useCallback } from 'react';
import { fetchHosts } from './api/client';
import { Dashboard } from './pages/Dashboard';
import { AddHostWizard } from './components/AddHostWizard';
import type { HostStatus } from './types';
import './App.css';

export default function App() {
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
        <div className="app-header__actions">
          {updateCount > 0 && (
            <span className="badge badge--warning">
              {updateCount} update{updateCount > 1 ? 's' : ''}
            </span>
          )}
          <span className="app-header__hosts">
            {hosts.length} host{hosts.length !== 1 ? 's' : ''}
          </span>
          <button className="btn-secondary" onClick={load} disabled={loading}>
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
          <button className="btn-primary" onClick={() => setShowWizard(true)}>
            + Add Host
          </button>
        </div>
      </header>
      <main>
        <Dashboard hosts={hosts} loading={loading} error={error} />
      </main>
      {showWizard && (
        <AddHostWizard onClose={() => { setShowWizard(false); load(); }} />
      )}
    </div>
  );
}
