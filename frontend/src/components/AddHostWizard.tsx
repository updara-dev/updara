import { useEffect, useState } from 'react';
import { fetchConnectors, createProvision, type ConnectorMeta } from '../api/client';

interface Props {
  onClose: () => void;
}

type Step = 1 | 2 | 3;

const HOST_TYPES = [
  { value: 'lxc',         label: 'LXC / VM',         desc: 'Proxmox LXC or any virtual machine' },
  { value: 'docker',      label: 'Docker Host',       desc: 'Server running only Docker containers' },
  { value: 'docker+host', label: 'Docker + Host',     desc: 'Monitors both the OS and Docker' },
  { value: 'custom',      label: 'Custom',            desc: 'Choose connectors manually' },
];

const DEFAULT_CONNECTORS: Record<string, string[]> = {
  lxc:          ['apt'],
  docker:       [],
  'docker+host': ['apt'],
  custom:       [],
};

export function AddHostWizard({ onClose }: Props) {
  const [step, setStep] = useState<Step>(1);
  const [name, setName] = useState('');
  const [hostType, setHostType] = useState('lxc');
  const [connectors, setConnectors] = useState<ConnectorMeta[]>([]);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [vars, setVars] = useState<Record<string, Record<string, string>>>({});
  const [installCmd, setInstallCmd] = useState('');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    fetchConnectors().then(setConnectors).catch(console.error);
  }, []);

  const handleTypeSelect = (type: string) => {
    setHostType(type);
    const defaults = DEFAULT_CONNECTORS[type] ?? [];
    const sel: Record<string, boolean> = {};
    defaults.forEach(n => sel[n] = true);
    setSelected(sel);
  };

  const toggleConnector = (name: string) => {
    setSelected(prev => ({ ...prev, [name]: !prev[name] }));
  };

  const setVar = (connector: string, key: string, value: string) => {
    setVars(prev => ({
      ...prev,
      [connector]: { ...(prev[connector] ?? {}), [key]: value },
    }));
  };

  const handleNext = async () => {
    if (step === 1) { setStep(2); return; }
    if (step === 2) {
      const specs = connectors
        .filter(c => selected[c.name])
        .map(c => ({ name: c.name, vars: vars[c.name] ?? {} }));
      try {
        const res = await createProvision({ name, host_type: hostType, connectors: specs });
        setInstallCmd(res.install_cmd);
        setStep(3);
      } catch (e) {
        alert('Error: ' + e);
      }
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(installCmd);
    } catch {
      // Fallback for HTTP (no secure context)
      const el = document.createElement('textarea');
      el.value = installCmd;
      el.style.cssText = 'position:fixed;opacity:0';
      document.body.appendChild(el);
      el.select();
      document.execCommand('copy');
      document.body.removeChild(el);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="wizard-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
      <div className="wizard">
        <div className="wizard-header">
          <div className="wizard-steps">
            {(['1', '2', '3'] as const).map((s, i) => (
              <div key={s} className={`wizard-step ${Number(s) <= step ? 'active' : ''}`}>
                <span>{s}</span>
                {i < 2 && <div className="wizard-step-line" />}
              </div>
            ))}
          </div>
          <button className="wizard-close" onClick={onClose}>✕</button>
        </div>

        {/* Step 1: Name + Type */}
        {step === 1 && (
          <div className="wizard-body">
            <h2>Add Host</h2>
            <p className="wizard-sub">Give the host a name and choose what to monitor.</p>
            <label>
              <span>Host name</span>
              <input
                className="wizard-input"
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="e.g. pihole-prod, proxmox-1"
                autoFocus
              />
            </label>
            <div className="wizard-types">
              {HOST_TYPES.map(t => (
                <button
                  key={t.value}
                  className={`type-card ${hostType === t.value ? 'selected' : ''}`}
                  onClick={() => handleTypeSelect(t.value)}
                >
                  <strong>{t.label}</strong>
                  <span>{t.desc}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Step 2: Connectors */}
        {step === 2 && (
          <div className="wizard-body">
            <h2>Choose Connectors</h2>
            <p className="wizard-sub">Select what to monitor on <strong>{name}</strong>.</p>
            <div className="connector-list">
              {connectors.map(c => (
                <div key={c.name} className={`connector-card ${selected[c.name] ? 'selected' : ''}`}>
                  <div className="connector-card-top" onClick={() => toggleConnector(c.name)}>
                    <div className="connector-card-check">{selected[c.name] ? '✅' : '☐'}</div>
                    <div>
                      <strong>{c.display_name || c.name}</strong>
                      <span className="connector-category">{c.category}</span>
                    </div>
                  </div>
                  {selected[c.name] && (c.vars ?? []).length > 0 && (
                    <div className="connector-vars">
                      {c.vars.map(v => (
                        <label key={v.name}>
                          <span>{v.name}{v.required && ' *'}</span>
                          {v.description && <small>{v.description}</small>}
                          <input
                            className="wizard-input"
                            value={vars[c.name]?.[v.name] ?? v.default ?? ''}
                            onChange={e => setVar(c.name, v.name, e.target.value)}
                            placeholder={v.default || v.name}
                          />
                        </label>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Step 3: Install command */}
        {step === 3 && (
          <div className="wizard-body">
            <h2>Run on <strong>{name}</strong></h2>
            <p className="wizard-sub">Copy and run this command on the target host (as root):</p>
            <div className="install-box">
              <code>{installCmd}</code>
              <button className="copy-btn" onClick={copy}>
                {copied ? '✓ Copied' : 'Copy'}
              </button>
            </div>
            <p className="wizard-hint">
              The agent will install itself, start automatically, and appear in the dashboard within a minute.
            </p>
          </div>
        )}

        <div className="wizard-footer">
          {step > 1 && step < 3 && (
            <button className="btn-secondary" onClick={() => setStep(s => (s - 1) as Step)}>
              Back
            </button>
          )}
          {step < 3 && (
            <button
              className="btn-primary"
              onClick={handleNext}
              disabled={step === 1 && !name.trim()}
            >
              {step === 2 ? 'Generate Install Command' : 'Next'}
            </button>
          )}
          {step === 3 && (
            <button className="btn-primary" onClick={onClose}>Done</button>
          )}
        </div>
      </div>
    </div>
  );
}
