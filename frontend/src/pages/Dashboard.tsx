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

function timeAgo(dateStr: string): string {
  const s = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

function HostListRow({ status, selected, onToggleSelect, onClick }: {
  status: HostStatus;
  selected: Set<string>;
  onToggleSelect: (hostname: string, connector: string) => void;
  onClick: () => void;
}) {
  const { host, results } = status;
  const updatable = (results ?? []).filter(r => r.update_available && !r.ignored);
  const errors = (results ?? []).filter(r => r.error && !r.ignored);
  const allChecked = updatable.length > 0 && updatable.every(r => selected.has(updateKey(host.hostname, r.connector)));
  const color = errors.length > 0 ? 'var(--color-critical)' : updatable.length > 0 ? 'var(--color-warning)' : 'var(--color-ok)';
  const secs = (Date.now() - new Date(host.last_seen).getTime()) / 1000;
  const agentStatus = secs < 300 ? 'online' : secs < 3600 ? 'stale' : 'offline';

  const toggleAll = (e: React.ChangeEvent<HTMLInputElement>) => {
    e.stopPropagation();
    updatable.forEach(r => onToggleSelect(host.hostname, r.connector));
  };

  return (
    <div className={`host-list-row host-list-row--${agentStatus}`} onClick={onClick}>
      <div className="host-list-row__check" onClick={e => e.stopPropagation()}>
        {updatable.length > 0 && (
          <input type="checkbox" checked={allChecked} onChange={toggleAll} />
        )}
      </div>
      <span className="host-list-row__dot" style={{ background: color }} />
      <span className="host-list-row__name">{host.display_name || host.hostname}</span>
      {host.ip_address && <span className="host-list-row__ip">{host.ip_address}</span>}
      <div className="host-list-row__badges">
        {errors.length > 0 && <span className="host-list-row__badge host-list-row__badge--error">✕ {errors.length}</span>}
        {updatable.length > 0 && <span className="host-list-row__badge host-list-row__badge--update">⚠ {updatable.length}</span>}
        {updatable.length === 0 && errors.length === 0 && <span className="host-list-row__badge host-list-row__badge--ok">✓</span>}
      </div>
      <span className={`host-list-row__agent host-list-row__agent--${agentStatus}`}>{agentStatus}</span>
      <span className="host-list-row__time">{timeAgo(host.last_seen)}</span>
    </div>
  );
}

export function Dashboard({ hosts, loading, error, onSelectHost, onAddHost }: Props) {
  const { t } = useT();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [triggered, setTriggered] = useState<number | null>(null);
  const [view, setView] = useState<'grid' | 'list'>(() =>
    (localStorage.getItem('dashboard-view') as 'grid' | 'list') ?? 'grid'
  );
  const [filterText, setFilterText] = useState('');
  const [filterStatus, setFilterStatus] = useState<Set<'updates' | 'errors'>>(new Set());

  const switchView = (v: 'grid' | 'list') => {
    setView(v);
    localStorage.setItem('dashboard-view', v);
  };

  const toggleStatusFilter = (s: 'updates' | 'errors') => {
    setFilterStatus(prev => {
      const next = new Set(prev);
      next.has(s) ? next.delete(s) : next.add(s);
      return next;
    });
  };

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

  const visibleHosts = useMemo(() => {
    let h = hosts;
    if (filterText.trim()) {
      const q = filterText.toLowerCase();
      h = h.filter(hs =>
        (hs.host.display_name || hs.host.hostname).toLowerCase().includes(q) ||
        (hs.host.ip_address ?? '').includes(q)
      );
    }
    if (filterStatus.has('updates')) h = h.filter(hs => (hs.results ?? []).some(r => r.update_available && !r.ignored));
    if (filterStatus.has('errors'))  h = h.filter(hs => (hs.results ?? []).some(r => r.error && !r.ignored));
    return h;
  }, [hosts, filterText, filterStatus]);

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
      <div className="dashboard-toolbar">
        <div className="dashboard-toolbar__left">
          {allUpdatable.length > 0 && (
            <button className="bulk-select-all-btn" onClick={handleSelectAll}>
              {allSelected ? t.dashboard.deselectAll : t.dashboard.selectAll}
            </button>
          )}
          <button
            className={`filter-pill${filterStatus.has('updates') ? ' filter-pill--active filter-pill--updates' : ''}`}
            onClick={() => toggleStatusFilter('updates')}
          >⚠ Updates</button>
          <button
            className={`filter-pill${filterStatus.has('errors') ? ' filter-pill--active filter-pill--errors' : ''}`}
            onClick={() => toggleStatusFilter('errors')}
          >✕ Errors</button>
        </div>
        <div className="dashboard-toolbar__right">
          <input
            className="filter-input"
            type="text"
            placeholder="Filter…"
            value={filterText}
            onChange={e => setFilterText(e.target.value)}
          />
          <div className="view-toggle">
            <button className={`view-toggle__btn${view === 'grid' ? ' view-toggle__btn--active' : ''}`} title="Card view" onClick={() => switchView('grid')}>⊞</button>
            <button className={`view-toggle__btn${view === 'list' ? ' view-toggle__btn--active' : ''}`} title="List view" onClick={() => switchView('list')}>☰</button>
          </div>
        </div>
      </div>

      {view === 'grid' ? (
        <div className="host-grid">
          {visibleHosts.map(h => (
            <HostCard
              key={h.host.id}
              status={h}
              selected={selected}
              onToggleSelect={toggleSelect}
              onClick={() => onSelectHost(h.host.hostname)}
            />
          ))}
        </div>
      ) : (
        <div className="host-list">
          {visibleHosts.map(h => (
            <HostListRow
              key={h.host.id}
              status={h}
              selected={selected}
              onToggleSelect={toggleSelect}
              onClick={() => onSelectHost(h.host.hostname)}
            />
          ))}
        </div>
      )}
      {visibleHosts.length === 0 && hosts.length > 0 && (
        <p className="status-msg">{filterText ? `No hosts matching "${filterText}"` : 'No hosts match the active filter.'}</p>
      )}

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
