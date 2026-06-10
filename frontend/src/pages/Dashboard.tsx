import { useState, useMemo } from 'react';
import type { HostStatus } from '../types';
import { HostCard } from '../components/HostCard';
import { triggerUpdate } from '../api/client';
import { useT } from '../i18n';

interface Props {
  hosts: HostStatus[];
  loading: boolean;
  error: string | null;
  onSelectHost: (hostname: string) => void;
  onAddHost: (initialName?: string) => void;
}

const updateKey = (hostname: string, connector: string) => `${hostname}::${connector}`;

export function Dashboard({ hosts, loading, error, onSelectHost, onAddHost }: Props) {
  const { t } = useT();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [triggered, setTriggered] = useState<number | null>(null);

  const allUpdatable = useMemo(() =>
    hosts.flatMap(h =>
      (h.results ?? [])
        .filter(r => r.update_available && !r.ignored)
        .map(r => updateKey(h.host.hostname, r.connector))
    ), [hosts]);

  const allSelected = allUpdatable.length > 0 && allUpdatable.every(k => selected.has(k));

  const toggleSelect = (hostname: string, connector: string) => {
    const key = updateKey(hostname, connector);
    setSelected(prev => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });
  };

  const handleSelectAll = () => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(allUpdatable));
    }
  };

  const handleBulkUpdate = async () => {
    const keys = [...selected];
    const count = keys.length;
    await Promise.all(
      keys.map(key => {
        const [hostname, connector] = key.split('::');
        return triggerUpdate(hostname, connector).catch(console.error);
      })
    );
    setSelected(new Set());
    setTriggered(count);
    setTimeout(() => setTriggered(null), 4000);
  };

  if (loading && hosts.length === 0) return <p className="status-msg">{t.dashboard.loading}</p>;
  if (error) return <p className="status-msg status-msg--error">{t.dashboard.error(error)}</p>;
  if (hosts.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-state__self-host">
          <div className="empty-state__icon">📡</div>
          <h3 className="empty-state__title">{t.dashboard.selfHostTitle}</h3>
          <p className="empty-state__hint">{t.dashboard.selfHostHint}</p>
          <button className="btn-primary" onClick={() => onAddHost('Updara')}>
            {t.dashboard.selfHostBtn}
          </button>
        </div>
        <p className="empty-state__or">{t.dashboard.noHostsHint}</p>
      </div>
    );
  }

  return (
    <>
      {allUpdatable.length > 0 && (
        <div className="bulk-controls">
          <button className="bulk-select-all-btn" onClick={handleSelectAll}>
            {allSelected ? t.dashboard.deselectAll : t.dashboard.selectAll}
          </button>
        </div>
      )}

      <div className="host-grid">
        {hosts.map(h => (
          <HostCard
            key={h.host.id}
            status={h}
            selected={selected}
            onToggleSelect={toggleSelect}
            onClick={() => onSelectHost(h.host.hostname)}
          />
        ))}
      </div>

      {(selected.size > 0 || triggered !== null) && (
        <div className="bulk-update-bar">
          {triggered !== null ? (
            <span className="bulk-update-bar__triggered">{t.dashboard.bulkTriggered(triggered)}</span>
          ) : (
            <>
              <span className="bulk-update-bar__count">{selected.size} selected</span>
              <button className="bulk-update-bar__clear" onClick={() => setSelected(new Set())}>✕</button>
              <button className="bulk-update-bar__btn" onClick={handleBulkUpdate}>
                {t.dashboard.bulkUpdate(selected.size)}
              </button>
            </>
          )}
        </div>
      )}
    </>
  );
}
