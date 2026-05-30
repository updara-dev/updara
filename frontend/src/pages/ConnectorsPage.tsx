import { useEffect, useState } from 'react';
import {
  fetchConnectors,
  fetchConnectorYAML,
  saveConnectorYAML,
  deleteConnector,
  type ConnectorMeta,
} from '../api/client';
import { useT } from '../i18n';

const NEW_CONNECTOR_TEMPLATE = `name: my-connector
display_name: My Connector
category: Custom
# docs: https://example.com/docs

# vars:
#   - name: API_KEY
#     description: API key for the service
#     required: true
#     default: ""

checks:
  - name: example-version
    run: echo "1.0.0"
    parse: |
      import json, sys
      version = sys.stdin.read().strip()
      print(json.dumps({"version": version, "update_available": False}))
`;

interface EditorState {
  name: string;
  yaml: string;
  isNew: boolean;
}

export function ConnectorsPage() {
  const { t } = useT();
  const [connectors, setConnectors] = useState<ConnectorMeta[]>([]);
  const [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    fetchConnectors()
      .then(setConnectors)
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const openEditor = async (name: string) => {
    try {
      const yaml = await fetchConnectorYAML(name);
      setEditor({ name, yaml, isNew: false });
      setSaveError('');
    } catch (e) {
      alert(t.connectors.loadError(String(e)));
    }
  };

  const openNew = () => {
    setEditor({ name: '', yaml: NEW_CONNECTOR_TEMPLATE, isNew: true });
    setSaveError('');
  };

  const handleSave = async () => {
    if (!editor) return;
    setSaving(true);
    setSaveError('');

    let targetName = editor.name;
    if (editor.isNew) {
      const match = editor.yaml.match(/^name:\s*(\S+)/m);
      targetName = match?.[1] ?? '';
      if (!targetName) {
        setSaveError(t.connectors.noNameError);
        setSaving(false);
        return;
      }
    }

    try {
      await saveConnectorYAML(targetName, editor.yaml);
      setEditor(null);
      load();
    } catch (e) {
      setSaveError(t.connectors.saveError(String(e)));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await deleteConnector(name);
      setConfirmDelete(null);
      load();
    } catch (e) {
      alert(t.connectors.deleteError(String(e)));
    }
  };

  const groups: Record<string, ConnectorMeta[]> = {};
  for (const c of connectors) {
    const cat = c.category || 'Other';
    if (!groups[cat]) groups[cat] = [];
    groups[cat].push(c);
  }

  return (
    <div className="connectors-page">
      <div className="connectors-toolbar">
        <div>
          <h2 className="connectors-title">{t.connectors.title}</h2>
          <p className="connectors-subtitle">{t.connectors.subtitle}</p>
        </div>
        <button className="btn-primary" onClick={openNew}>{t.connectors.newConnector}</button>
      </div>

      {loading && <p className="status-msg">{t.connectors.loading}</p>}

      {!loading && connectors.length === 0 && (
        <p className="status-msg">{t.connectors.empty}</p>
      )}

      {!loading && Object.entries(groups).map(([category, items]) => (
        <div key={category} className="connectors-group">
          <h3 className="connectors-group-label">{category}</h3>
          <div className="connectors-table">
            {items.map(c => (
              <div key={c.name} className="connectors-row">
                <div className="connectors-row-main">
                  <span className="connectors-row-name">{c.display_name || c.name}</span>
                  <span className="connectors-row-id">{c.name}.yaml</span>
                </div>
                <div className="connectors-row-meta">
                  {(c.vars ?? []).length > 0 && (
                    <span className="connectors-row-vars">{t.connectors.vars(c.vars.length)}</span>
                  )}
                </div>
                <div className="connectors-row-actions">
                  <button className="btn-secondary" onClick={() => openEditor(c.name)}>
                    {t.connectors.editYaml}
                  </button>
                  <button className="btn-danger" onClick={() => setConfirmDelete(c.name)}>
                    {t.connectors.delete}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}

      {editor && (
        <div className="yaml-overlay" onClick={e => e.target === e.currentTarget && setEditor(null)}>
          <div className="yaml-editor">
            <div className="yaml-editor-header">
              <span className="yaml-editor-title">
                {editor.isNew ? t.connectors.newConnectorTitle : `${editor.name}.yaml`}
              </span>
              <button className="wizard-close" onClick={() => setEditor(null)}>✕</button>
            </div>
            <textarea
              className="yaml-editor-area"
              value={editor.yaml}
              onChange={e => setEditor(prev => prev ? { ...prev, yaml: e.target.value } : null)}
              spellCheck={false}
              autoCorrect="off"
              autoCapitalize="off"
            />
            {saveError && <div className="yaml-editor-error">{saveError}</div>}
            <div className="yaml-editor-footer">
              <button className="btn-secondary" onClick={() => setEditor(null)}>{t.connectors.cancel}</button>
              <button className="btn-primary" onClick={handleSave} disabled={saving}>
                {saving ? t.connectors.saving : t.connectors.save}
              </button>
            </div>
          </div>
        </div>
      )}

      {confirmDelete && (
        <div className="yaml-overlay" onClick={() => setConfirmDelete(null)}>
          <div className="confirm-dialog" onClick={e => e.stopPropagation()}>
            <p>{t.connectors.confirmDelete(confirmDelete)}</p>
            <p className="confirm-hint">{t.connectors.confirmDeleteHint}</p>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={() => setConfirmDelete(null)}>{t.connectors.cancel}</button>
              <button className="btn-danger" onClick={() => handleDelete(confirmDelete)}>{t.connectors.delete}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
