import { useEffect, useState } from 'react';
import type { CheckResult, Command } from '../types';
import { triggerUpdate, fetchCommands } from '../api/client';
import { useT } from '../i18n';

interface Props {
  result: CheckResult;
  hostname: string;
}

function stripAnsi(str: string): string {
  return str.replace(/\x1b\[[0-9;?]*[a-zA-Z]|\r/g, '').trim();
}

export function CheckRow({ result, hostname }: Props) {
  const { t } = useT();
  const [cmd, setCmd] = useState<Command | null>(null);
  const [showOutput, setShowOutput] = useState(false);

  useEffect(() => {
    if (!cmd || cmd.status === 'done' || cmd.status === 'failed') return;
    const id = setInterval(async () => {
      const cmds = await fetchCommands(hostname).catch(() => [] as Command[]);
      const latest = cmds.find(c => c.id === cmd.id);
      if (latest) {
        setCmd(latest);
        if (latest.status === 'done' || latest.status === 'failed') {
          setShowOutput(true);
        }
      }
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

  const state = result.error ? 'error' : result.update_available ? 'update' : 'ok';
  const icon  = state === 'error' ? '❌' : state === 'update' ? '⚠️' : '✅';

  const valueLabel = Object.entries(result.values ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join(' · ');

  const statusBadge = () => {
    if (!cmd) return null;
    if (cmd.status === 'pending') return <span className="cmd-status pending">{t.checkRow.pending}</span>;
    if (cmd.status === 'running') return <span className="cmd-status running">{t.checkRow.running}</span>;
    if (cmd.status === 'done')
      return (
        <button className="cmd-status done toggle-output" onClick={() => setShowOutput(v => !v)}>
          {t.checkRow.updated} {showOutput ? '▲' : '▼'}
        </button>
      );
    if (cmd.status === 'failed')
      return (
        <button className="cmd-status failed toggle-output" onClick={() => setShowOutput(v => !v)}>
          {t.checkRow.failed} {showOutput ? '▲' : '▼'}
        </button>
      );
  };

  return (
    <div className="check-row-wrapper">
      <div className={`check-row check-row--${state}`}>
        <span className="check-row__icon">{icon}</span>
        <div className="check-row__body">
          <span className="check-row__name">{result.display_name || result.connector}</span>
          {result.error && (
            <span className="check-row__detail check-row__detail--error">{result.error}</span>
          )}
          {!result.error && valueLabel && (
            <span className="check-row__detail">{valueLabel}</span>
          )}
        </div>
        <div className="check-row__actions">
          {statusBadge()}
          {result.update_available && !cmd && (
            <button className="update-btn" onClick={handleUpdate}>{t.checkRow.update}</button>
          )}
          {result.update_available && result.changelog && !cmd && (
            <a className="check-row__changelog" href={result.changelog}
               target="_blank" rel="noopener noreferrer">
              {t.checkRow.changelog}
            </a>
          )}
        </div>
      </div>
      {showOutput && cmd?.output && (
        <pre className="cmd-output">{stripAnsi(cmd.output)}</pre>
      )}
    </div>
  );
}
