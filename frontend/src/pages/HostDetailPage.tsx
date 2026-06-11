import { useEffect, useState, useCallback } from 'react';
import { fetchHostDetail, ignoreConnector, unignoreConnector, triggerUpdate, fetchCommands, removeHostConnector, deleteHost, fetchHostProvision, updateProvision, recheckConnector, fetchConnectors, syncAgent, installConnector, fetchHostStats } from '../api/client';
import type { ConnectorMeta, UpdateRecord } from '../api/client';
import { useT } from '../i18n';
import type { HostDetail, CheckResult, Command, Provision } from '../types';

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

function VarsEditDialog({
  hostname,
  provision,
  connectorMeta,
  onClose,
}: {
  hostname: string;
  provision: Provision;
  connectorMeta: ConnectorMeta[];
  onClose: () => void;
}) {
  const { t } = useT();
  const [vars, setVars] = useState<Record<string, Record<string, string>>>(() => {
    const m: Record<string, Record<string, string>> = {};
    for (const c of provision.connectors) {
      m[c.name] = { ...c.vars };
    }
    return m;
  });
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [errMsg, setErrMsg] = useState('');

  // Connectors that have var definitions AND are in the provision
  const editableConnectors = connectorMeta.filter(
    meta => meta.vars.length > 0 && provision.connectors.some(c => c.name === meta.name)
  );

  const handleSave = async () => {
    setStatus('saving');
    try {
      const connectors = provision.connectors.map(c => ({
        name: c.name,
        vars: vars[c.name] ?? {},
      }));
      await updateProvision(provision.token, connectors);
      setStatus('saved');
      setTimeout(onClose, 1800);
    } catch (e) {
      setErrMsg(String(e));
      setStatus('error');
    }
  };

  return (
    <div className="vars-dialog-overlay" onClick={onClose}>
      <div className="vars-dialog" onClick={e => e.stopPropagation()}>
        <div className="vars-dialog__header">
          <h3>{t.hostDetail.editVarsTitle(hostname)}</h3>
          <button className="vars-dialog__close" onClick={onClose}>✕</button>
        </div>
        <div className="vars-dialog__body">
          {editableConnectors.length === 0 ? (
            <p className="vars-dialog__empty">{t.hostDetail.noVars}</p>
          ) : (
            editableConnectors.map(meta => (
              <div key={meta.name} className="vars-dialog__section">
                <h4 className="vars-dialog__section-title">{meta.display_name}</h4>
                {meta.vars.map(v => (
                  <div key={v.name} className="vars-dialog__field">
                    <label className="vars-dialog__label">
                      <span className="vars-dialog__var-name">{v.name}</span>
                      {v.description && <span className="vars-dialog__var-desc">{v.description}</span>}
                    </label>
                    <input
                      className="vars-dialog__input"
                      type="text"
                      value={vars[meta.name]?.[v.name] ?? ''}
                      placeholder={v.default || ''}
                      onChange={e => setVars(prev => ({
                        ...prev,
                        [meta.name]: { ...(prev[meta.name] ?? {}), [v.name]: e.target.value },
                      }))}
                    />
                  </div>
                ))}
              </div>
            ))
          )}
        </div>
        <div className="vars-dialog__footer">
          {status === 'saved' && <span className="vars-dialog__status ok">{t.hostDetail.varsUpdated}</span>}
          {status === 'error' && <span className="vars-dialog__status err">{t.hostDetail.varsError(errMsg)}</span>}
          <button className="vars-dialog__cancel" onClick={onClose}>{status === 'saved' ? 'Close' : 'Cancel'}</button>
          {status !== 'saved' && (
            <button className="vars-dialog__save" onClick={handleSave} disabled={status === 'saving'}>
              {status === 'saving' ? t.hostDetail.savingVars : t.hostDetail.saveVars}
            </button>
          )}
        </div>
      </div>
    </div>
  );
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
  onRemove,
  onCommandComplete,
}: {
  result: CheckResult;
  hostname: string;
  onToggleIgnore: (connector: string, item: string | undefined, ignored: boolean) => Promise<void>;
  onRemove: (connector: string) => Promise<void>;
  onCommandComplete: () => void;
}) {
  const { t } = useT();
  const [busy, setBusy] = useState(false);
  const [cmd, setCmd] = useState<Command | null>(null);
  const [rescanning, setRescanning] = useState(false);

  useEffect(() => {
    if (!cmd) return;
    if (cmd.status === 'done' || cmd.status === 'failed') {
      onCommandComplete();
      return;
    }
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

  const handleRescan = async () => {
    setRescanning(true);
    await recheckConnector(hostname, result.connector).catch(() => {});
    setTimeout(() => setRescanning(false), 3000);
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
            <button
              className="rescan-btn"
              onClick={handleRescan}
              disabled={rescanning}
            >{rescanning ? t.hostDetail.rescanQueued : t.hostDetail.rescan}</button>
            <button
              className="remove-connector-btn"
              title={t.hostDetail.removeConnector}
              onClick={() => {
                if (window.confirm(t.hostDetail.confirmRemoveConnector(result.display_name || result.connector))) {
                  onRemove(result.connector);
                }
              }}
            >✕</button>
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
            <button
              className="remove-connector-btn"
              title={t.hostDetail.removeConnector}
              onClick={() => {
                if (window.confirm(t.hostDetail.confirmRemoveConnector(result.display_name || result.connector))) {
                  onRemove(result.connector);
                }
              }}
            >✕</button>
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

function AddConnectorPanel({
  hostname,
  available,
  onDone,
}: {
  hostname: string;
  available: ConnectorMeta[];
  onDone: () => void;
}) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState('');
  const [status, setStatus] = useState<'idle' | 'queued' | 'done' | 'error'>('idle');
  const [errMsg, setErrMsg] = useState('');

  const handleInstall = async () => {
    if (!selected) return;
    setStatus('queued');
    try {
      await installConnector(hostname, selected);
      setStatus('done');
      setTimeout(() => { setStatus('idle'); setSelected(''); setOpen(false); onDone(); }, 3000);
    } catch (e) {
      setErrMsg(String(e));
      setStatus('error');
    }
  };

  if (!open) {
    return (
      <button className="add-connector-btn" onClick={() => setOpen(true)}>
        {t.hostDetail.addConnector}
      </button>
    );
  }

  return (
    <div className="add-connector-panel">
      {available.length === 0 ? (
        <span className="add-connector-panel__empty">{t.hostDetail.noMoreConnectors}</span>
      ) : (
        <>
          <select
            className="add-connector-panel__select"
            value={selected}
            onChange={e => setSelected(e.target.value)}
          >
            <option value="">— Connector wählen —</option>
            {available.map(c => (
              <option key={c.name} value={c.name}>{c.display_name || c.name}</option>
            ))}
          </select>
          <button
            className="btn-primary"
            onClick={handleInstall}
            disabled={!selected || status === 'queued'}
          >
            {status === 'queued' ? t.hostDetail.addConnectorQueued : t.hostDetail.addConnectorBtn}
          </button>
        </>
      )}
      {status === 'done' && <span className="add-connector-panel__status ok">{t.hostDetail.addConnectorDone}</span>}
      {status === 'error' && <span className="add-connector-panel__status err">{t.hostDetail.addConnectorError(errMsg)}</span>}
      <button className="add-connector-panel__cancel" onClick={() => { setOpen(false); setStatus('idle'); }}>✕</button>
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
  const [provision, setProvision] = useState<Provision | null>(null);
  const [connectorMeta, setConnectorMeta] = useState<ConnectorMeta[]>([]);
  const [showVarsDialog, setShowVarsDialog] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [updateHistory, setUpdateHistory] = useState<UpdateRecord[]>([]);

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
    fetchHostProvision(hostname).then(setProvision).catch(() => {});
    fetchConnectors().then(setConnectorMeta).catch(() => {});
    fetchHostStats(hostname).then(setUpdateHistory).catch(() => {});
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

  const handleRemove = async (connector: string) => {
    await removeHostConnector(hostname, connector);
    await load();
  };

  const handleSync = async () => {
    setSyncing(true);
    await syncAgent(hostname).catch(console.error);
    setTimeout(() => { setSyncing(false); load(); }, 15_000);
  };

  const handleDeleteHost = async () => {
    if (!window.confirm(t.hostDetail.confirmDeleteHost(hostname))) return;
    await deleteHost(hostname);
    onBack();
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
      {showVarsDialog && provision && (
        <VarsEditDialog
          hostname={hostname}
          provision={provision}
          connectorMeta={connectorMeta}
          onClose={() => {
            setShowVarsDialog(false);
            fetchHostProvision(hostname).then(setProvision).catch(() => {});
          }}
        />
      )}
      <div className="host-detail__topbar">
        <button className="host-detail__back" onClick={onBack}>
          {t.hostDetail.back}
        </button>
        <div className="host-detail__topbar-actions">
          <button className="sync-agent-btn" onClick={handleSync} disabled={syncing}>
            {syncing ? t.hostDetail.syncAgentQueued : t.hostDetail.syncAgent}
          </button>
          {provision && (
            <button className="edit-vars-btn" onClick={() => setShowVarsDialog(true)}>
              {t.hostDetail.editVars}
            </button>
          )}
          <button className="delete-host-btn" onClick={handleDeleteHost}>
            {t.hostDetail.deleteHost}
          </button>
        </div>
      </div>

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
                onRemove={handleRemove}
                onCommandComplete={load}
              />
            ))}
          </div>
        ))}
      </div>

      <div className="host-detail__add-connector">
        <AddConnectorPanel
          hostname={hostname}
          available={connectorMeta.filter(c => !results.some(r => r.connector === c.name))}
          onDone={load}
        />
      </div>

      {updateHistory.length > 0 && (
        <div className="update-history">
          <h3 className="update-history__title">{t.stats.updateHistory}</h3>
          <table className="update-history__table">
            <tbody>
              {updateHistory.slice(0, 15).map((rec, i) => (
                <tr key={i} className={`update-history__row update-history__row--${rec.status}`}>
                  <td className="update-history__connector">{rec.display_name || rec.connector}</td>
                  <td className="update-history__status">{
                    rec.status === 'done' ? t.stats.statusDone :
                    rec.status === 'failed' ? t.stats.statusFailed :
                    rec.status === 'running' ? t.stats.statusRunning :
                    t.stats.statusPending
                  }</td>
                  <td className="update-history__date">{new Date(rec.updated_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

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
