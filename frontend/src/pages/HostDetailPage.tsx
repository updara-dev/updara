import { useEffect, useState, useCallback } from 'react';
import { fetchHostDetail, ignoreConnector, unignoreConnector, triggerUpdate, fetchCommands } from '../api/client';
import { useT } from '../i18n';
import type { HostDetail, CheckResult, Command } from '../types';

interface Props {
  hostname: string;
  onBack: () => void;
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

function agentStatus(lastSeen: string): 'online' | 'stale' | 'offline' {
  const seconds = (Date.now() - new Date(lastSeen).getTime()) / 1000;
  if (seconds < 300) return 'online';
  if (seconds < 3600) return 'stale';
  return 'offline';
}

function stripAnsi(str: string): string {
  return str.replace(/\x1b\[[0-9;?]*[a-zA-Z]|\r/g, '').trim();
}

function ContainerRow({
  name,
  isIgnored,
  isStandalone,
  onToggle,
}: {
  name: string;
  isIgnored: boolean;
  isStandalone: boolean;
  onToggle: () => Promise<void>;
}) {
  const { t } = useT();
  const [busy, setBusy] = useState(false);
  const handle = async () => { setBusy(true); await onToggle(); setBusy(false); };
  return (
    <div className={`detail-row__container${isIgnored ? ' detail-row__container--ignored' : ' detail-row__container--update'}`}>
      <span className="detail-row__container-icon">{isIgnored ? '🔕' : '⚠️'}</span>
      <span className="detail-row__container-name">{name}</span>
      {isStandalone && !isIgnored && (
        <span className="container-standalone-badge">manual</span>
      )}
      <button
        className={isIgnored ? 'unignore-btn' : 'ignore-btn'}
        onClick={handle}
        disabled={busy}
      >
        {isIgnored ? t.hostDetail.unignore : t.hostDetail.ignore}
      </button>
    </div>
  );
}

function ResultRow({
  result,
  hostname,
  onToggleIgnore,
}: {
  result: CheckResult;
  hostname: string;
  onToggleIgnore: (connector: string, item: string | undefined, ignored: boolean) => Promise<void>;
}) {
  const { t } = useT();
  const [busy, setBusy] = useState(false);
  const [cmd, setCmd] = useState<Command | null>(null);

  useEffect(() => {
    if (!cmd || cmd.status === 'done' || cmd.status === 'failed') return;
    const id = setInterval(async () => {
      const cmds = await fetchCommands(hostname).catch(() => [] as Command[]);
      const latest = cmds.find(c => c.id === cmd.id);
      if (latest) setCmd(latest);
    }, 3000);
    return () => clearInterval(id);
  }, [cmd, hostname]);

  const handleUpdate = async () => {
    try {
      const created = await triggerUpdate(hostname, result.connector);
      setCmd(created);
    } catch (e) {
      alert(t.checkRow.updateError(String(e)));
    }
  };

  const outdatedContainers: string[] = result.values?.outdated
    ? result.values.outdated.split(',').map(s => s.trim()).filter(Boolean)
    : [];
  const ignoredContainers: string[] = result.ignored_items ?? [];
  const standaloneSet = new Set<string>(
    result.values?.standalone
      ? result.values.standalone.split(',').map(s => s.trim()).filter(Boolean)
      : []
  );
  const hasContainerView = outdatedContainers.length > 0 || ignoredContainers.length > 0;
  const hasComposeUpdates = outdatedContainers.some(n => !standaloneSet.has(n));

  const state = result.ignored ? 'ignored' : result.error ? 'error' : result.update_available ? 'update' : 'ok';
  const icon = result.ignored ? '🔕' : state === 'error' ? '❌' : state === 'update' ? '⚠️' : '✅';

  const valueLabel = Object.entries(result.values ?? {})
    .filter(([k]) => k !== 'outdated' || !hasContainerView)
    .map(([k, v]) => `${k}: ${v}`)
    .join(' · ');

  const handleConnectorToggle = async () => {
    setBusy(true);
    await onToggleIgnore(result.connector, undefined, !result.ignored);
    setBusy(false);
  };

  return (
    <div className="detail-row">
      <div className={`detail-row__main detail-row__main--${state}`}>
        <span className="detail-row__icon">{icon}</span>
        <div className="detail-row__body">
          <span className="detail-row__name">{result.display_name || result.connector}</span>
          {result.ignored && (
            <span className="detail-row__detail detail-row__detail--muted">{t.hostDetail.ignoredLabel}</span>
          )}
          {!result.ignored && result.error && (
            <span className="detail-row__detail detail-row__detail--error">{result.error}</span>
          )}
          {!result.ignored && !result.error && !hasContainerView && valueLabel && (
            <span className="detail-row__detail">{valueLabel}</span>
          )}
          {!result.ignored && hasContainerView && result.values?.count && (
            <span className="detail-row__detail">count: {result.values.count}</span>
          )}
        </div>
        {hasContainerView && (
          <div className="detail-row__actions">
            {cmd && cmd.status === 'pending' && <span className="cmd-status pending">{t.checkRow.pending}</span>}
            {cmd && cmd.status === 'running' && <span className="cmd-status running">{t.checkRow.running}</span>}
            {cmd && cmd.status === 'done' && <span className="cmd-status done">{t.checkRow.updated}</span>}
            {cmd && cmd.status === 'failed' && <span className="cmd-status failed">{t.checkRow.failed}</span>}
            {hasComposeUpdates && !cmd && (
              <button className="update-btn" onClick={handleUpdate}>{t.checkRow.update}</button>
            )}
          </div>
        )}
        {!hasContainerView && (
          <div className="detail-row__actions">
            {cmd && cmd.status === 'pending' && <span className="cmd-status pending">{t.checkRow.pending}</span>}
            {cmd && cmd.status === 'running' && <span className="cmd-status running">{t.checkRow.running}</span>}
            {cmd && cmd.status === 'done' && <span className="cmd-status done">{t.checkRow.updated}</span>}
            {cmd && cmd.status === 'failed' && <span className="cmd-status failed">{t.checkRow.failed}</span>}
            {result.update_available && !cmd && (
              <button className="update-btn" onClick={handleUpdate}>{t.checkRow.update}</button>
            )}
            <button
              className={result.ignored ? 'unignore-btn' : 'ignore-btn'}
              onClick={handleConnectorToggle}
              disabled={busy}
            >
              {result.ignored ? t.hostDetail.unignore : t.hostDetail.ignore}
            </button>
          </div>
        )}
      </div>

      {hasContainerView && (
        <div className="detail-row__containers">
          {outdatedContainers.map(name => (
            <ContainerRow
              key={name}
              name={name}
              isIgnored={false}
              isStandalone={standaloneSet.has(name)}
              onToggle={() => onToggleIgnore(result.connector, name, true)}
            />
          ))}
          {ignoredContainers.map(name => (
            <ContainerRow
              key={name}
              name={name}
              isIgnored={true}
              isStandalone={standaloneSet.has(name)}
              onToggle={() => onToggleIgnore(result.connector, name, false)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function CommandItem({ cmd }: { cmd: Command }) {
  const [open, setOpen] = useState(false);

  const statusClass =
    cmd.status === 'done' ? 'done' :
    cmd.status === 'failed' ? 'failed' :
    cmd.status === 'running' ? 'running' : 'pending';

  const statusLabel =
    cmd.status === 'done' ? '✅ done' :
    cmd.status === 'failed' ? '❌ failed' :
    cmd.status === 'running' ? '⏳ running' : '⏳ pending';

  return (
    <div className="command-item">
      <div className="command-item__header" onClick={() => setOpen(v => !v)}>
        <span className="command-item__connector">{cmd.connector}</span>
        <span className="command-item__time">{timeAgo(cmd.created_at)}</span>
        <span className={`command-item__status command-item__status--${statusClass}`}>{statusLabel}</span>
        <span className="command-item__toggle">{open ? '▲' : '▼'}</span>
      </div>
      {open && (
        <pre className="command-item__output">
          {cmd.output ? stripAnsi(cmd.output) : '(no output)'}
        </pre>
      )}
    </div>
  );
}

export function HostDetailPage({ hostname, onBack }: Props) {
  const { t } = useT();
  const [detail, setDetail] = useState<HostDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const d = await fetchHostDetail(hostname);
      setDetail(d);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, [hostname]);

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const handleToggleIgnore = async (connector: string, item: string | undefined, shouldIgnore: boolean) => {
    if (shouldIgnore) {
      await ignoreConnector(hostname, connector, item);
    } else {
      await unignoreConnector(hostname, connector, item);
    }
    await load();
  };

  if (!detail && !error) {
    return <p className="status-msg">Loading…</p>;
  }
  if (error) {
    return <p className="status-msg status-msg--error">Error: {error}</p>;
  }
  if (!detail) return null;

  const { host, results, commands } = detail;
  const status = agentStatus(host.last_seen);
  const statusColor =
    status === 'online' ? 'var(--color-ok)' :
    status === 'stale' ? 'var(--color-warning)' :
    'var(--color-critical)';
  const statusLabel =
    status === 'online' ? t.hostDetail.agentOnline :
    status === 'stale' ? t.hostDetail.agentStale :
    t.hostDetail.agentOffline;

  const byCategory = results.reduce<Record<string, CheckResult[]>>((acc, r) => {
    const cat = r.category || 'other';
    (acc[cat] ??= []).push(r);
    return acc;
  }, {});

  return (
    <div className="host-detail">
      <button className="host-detail__back" onClick={onBack}>
        {t.hostDetail.back}
      </button>

      <div className="host-detail__header">
        <div className="host-detail__title">
          <span className="host-detail__status-dot" style={{ background: statusColor }} />
          <h2>{host.hostname}</h2>
          {host.ip_address && <span className="host-detail__ip">{host.ip_address}</span>}
        </div>
        <div className="host-detail__meta">
          <span style={{ color: statusColor }}>{statusLabel}</span>
          {' · '}agent v{host.agent_version}
          {' · '}{t.hostDetail.lastSeen(timeAgo(host.last_seen))}
        </div>
      </div>

      <div className="host-detail__sections">
        {Object.entries(byCategory).map(([cat, items]) => (
          <div key={cat} className="host-detail__section">
            <h3 className="host-detail__section-title">{cat}</h3>
            {items.map(r => (
              <ResultRow
                key={r.connector}
                result={r}
                hostname={hostname}
                onToggleIgnore={handleToggleIgnore}
              />
            ))}
          </div>
        ))}
      </div>

      <div className="command-history">
        <h3 className="command-history__title">{t.hostDetail.commandHistory}</h3>
        {commands.length === 0 ? (
          <p className="host-detail__empty">{t.hostDetail.noCommands}</p>
        ) : (
          commands.slice(0, 20).map(cmd => <CommandItem key={cmd.id} cmd={cmd} />)
        )}
      </div>
    </div>
  );
}
