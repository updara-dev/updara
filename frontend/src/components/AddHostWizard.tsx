import { useEffect, useMemo, useState } from 'react';
import { fetchConnectors, createProvision, type ConnectorMeta } from '../api/client';
import { useT } from '../i18n';

interface Props {
  onClose: () => void;
}

const SERVICE_TEMPLATES: { keywords: string[]; label: string; connectors: string[] }[] = [
  { keywords: ['pihole', 'pi-hole'],              label: 'Pi-hole',         connectors: ['apt', 'system', 'pihole'] },
  { keywords: ['adguard', 'adguardhome'],          label: 'AdGuard Home',    connectors: ['apt', 'system'] },
  { keywords: ['proxmox', 'pve'],                  label: 'Proxmox VE',      connectors: ['apt', 'system'] },
  { keywords: ['homeassistant', 'home-assistant'], label: 'Home Assistant',  connectors: ['system'] },
  { keywords: ['nginx', 'caddy', 'traefik'],       label: 'Reverse Proxy',   connectors: ['apt', 'system'] },
  { keywords: ['n8n'],                              label: 'n8n',             connectors: ['system', 'n8n'] },
  { keywords: ['docker'],                          label: 'Docker Host',     connectors: ['system', 'docker-images'] },
  { keywords: ['debian', 'ubuntu', 'server'],      label: 'Linux Server',    connectors: ['apt', 'system'] },
];

function detectTemplate(name: string) {
  const lower = name.toLowerCase();
  return SERVICE_TEMPLATES.find(t => t.keywords.some(k => lower.includes(k))) ?? null;
}

function groupByCategory(connectors: ConnectorMeta[]) {
  const groups: Record<string, ConnectorMeta[]> = {};
  for (const c of connectors) {
    const cat = c.category || 'Other';
    if (!groups[cat]) groups[cat] = [];
    groups[cat].push(c);
  }
  return groups;
}

export function AddHostWizard({ onClose }: Props) {
  const { t } = useT();
  const [step, setStep] = useState<1 | 2>(1);
  const [name, setName] = useState('');
  const [connectors, setConnectors] = useState<ConnectorMeta[]>([]);
  const [selected, setSelected] = useState<Record<string, boolean>>({ apt: true, system: true });
  const [vars, setVars] = useState<Record<string, Record<string, string>>>({});
  const [search, setSearch] = useState('');
  const [installCmd, setInstallCmd] = useState('');
  const [copied, setCopied] = useState(false);
  const [lastTemplate, setLastTemplate] = useState<string | null>(null);
  const [advancedOpen, setAdvancedOpen] = useState<Record<string, boolean>>({});

  useEffect(() => {
    fetchConnectors().then(setConnectors).catch(console.error);
  }, []);

  useEffect(() => {
    const tmpl = detectTemplate(name);
    if (tmpl && tmpl.label !== lastTemplate) {
      setLastTemplate(tmpl.label);
      const sel: Record<string, boolean> = {};
      tmpl.connectors.forEach(n => { sel[n] = true; });
      setSelected(sel);
    } else if (!tmpl && lastTemplate) {
      setLastTemplate(null);
    }
  }, [name]);

  const template = detectTemplate(name);
  const templateConnectors = new Set(template?.connectors ?? []);

  const filtered = useMemo(() => {
    if (!search.trim()) return connectors;
    const q = search.toLowerCase();
    return connectors.filter(c =>
      c.name.includes(q) ||
      (c.display_name ?? '').toLowerCase().includes(q) ||
      (c.category ?? '').toLowerCase().includes(q),
    );
  }, [connectors, search]);

  const groups = useMemo(() => groupByCategory(filtered), [filtered]);

  const toggleConnector = (n: string) =>
    setSelected(prev => ({ ...prev, [n]: !prev[n] }));

  const setVar = (connector: string, key: string, value: string) =>
    setVars(prev => ({ ...prev, [connector]: { ...(prev[connector] ?? {}), [key]: value } }));

  const handleGenerate = async () => {
    const specs = connectors
      .filter(c => selected[c.name])
      .map(c => ({ name: c.name, vars: vars[c.name] ?? {} }));
    try {
      const res = await createProvision({ name, host_type: 'custom', connectors: specs });
      setInstallCmd(res.install_cmd);
      setStep(2);
    } catch (e) {
      alert('Error: ' + e);
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(installCmd);
    } catch {
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
            {([1, 2] as const).map((s, i) => (
              <div key={s} className={`wizard-step ${s <= step ? 'active' : ''}`}>
                <span>{s}</span>
                {i < 1 && <div className="wizard-step-line" />}
              </div>
            ))}
          </div>
          <button className="wizard-close" onClick={onClose}>✕</button>
        </div>

        {step === 1 && (
          <div className="wizard-body">
            <h2>{t.wizard.title}</h2>

            <label>
              <span>{t.wizard.hostNameLabel}</span>
              <input
                className="wizard-input"
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder={t.wizard.hostNamePlaceholder}
                autoFocus
              />
            </label>

            {template && (
              <div className="wizard-template-hint">
                <span className="wizard-template-dot" />
                <span>{t.wizard.detected(template.label)}</span>
              </div>
            )}

            <div className="wizard-connectors-header">
              <span className="wizard-section-label">{t.wizard.connectorsLabel}</span>
              <input
                className="wizard-search"
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder={t.wizard.search}
              />
            </div>

            <div className="connector-list">
              {Object.entries(groups).map(([category, items]) => (
                <div key={category} className="connector-group">
                  <div className="connector-group-label">{category}</div>
                  {items.map(c => (
                    <div key={c.name} className={`connector-card ${selected[c.name] ? 'selected' : ''}`}>
                      <div className="connector-card-top" onClick={() => toggleConnector(c.name)}>
                        <div className="connector-card-check">{selected[c.name] ? '✅' : '☐'}</div>
                        <div className="connector-card-info">
                          <strong>{c.display_name || c.name}</strong>
                          {templateConnectors.has(c.name) && (
                            <span className="connector-recommended">{t.wizard.recommended}</span>
                          )}
                        </div>
                      </div>
                      {selected[c.name] && (c.vars ?? []).length > 0 && (() => {
                        const requiredVars = c.vars.filter(v => v.required);
                        const optionalVars = c.vars.filter(v => !v.required);
                        const isOpen = advancedOpen[c.name] ?? false;
                        const renderVar = (v: typeof c.vars[0]) => (
                          <label key={v.name}>
                            <span>{v.name}{v.required && ' *'}</span>
                            {v.description && <small>{v.description}</small>}
                            <input
                              className="wizard-input"
                              value={vars[c.name]?.[v.name] ?? ''}
                              onChange={e => setVar(c.name, v.name, e.target.value)}
                              placeholder={v.default || v.name}
                            />
                          </label>
                        );
                        return (
                          <div className="connector-vars">
                            {requiredVars.map(renderVar)}
                            {optionalVars.length > 0 && (
                              <>
                                <button
                                  className="connector-vars-advanced-btn"
                                  onClick={e => { e.stopPropagation(); setAdvancedOpen(p => ({ ...p, [c.name]: !p[c.name] })); }}
                                >
                                  Advanced {isOpen ? '▲' : '▼'}
                                </button>
                                {isOpen && optionalVars.map(renderVar)}
                              </>
                            )}
                          </div>
                        );
                      })()}
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="wizard-body">
            <h2>{t.wizard.runOnHost(name)}</h2>
            <p className="wizard-sub">{t.wizard.installSubtitle}</p>
            <div className="install-box">
              <code>{installCmd}</code>
              <button className="copy-btn" onClick={copy}>
                {copied ? t.wizard.copied : t.wizard.copy}
              </button>
            </div>
            <p className="wizard-hint">{t.wizard.agentHint}</p>
            <div className="wizard-info-box">
              <strong>{t.wizard.tipTitle}</strong> {t.wizard.tipText}
            </div>
          </div>
        )}

        <div className="wizard-footer">
          {step === 2 && (
            <button className="btn-secondary" onClick={() => setStep(1)}>{t.wizard.back}</button>
          )}
          {step === 1 && (
            <button className="btn-primary" onClick={handleGenerate} disabled={!name.trim()}>
              {t.wizard.generate}
            </button>
          )}
          {step === 2 && (
            <button className="btn-primary" onClick={onClose}>{t.wizard.done}</button>
          )}
        </div>
      </div>
    </div>
  );
}
