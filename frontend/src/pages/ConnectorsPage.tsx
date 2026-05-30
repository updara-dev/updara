import { useEffect, useState } from 'react';
import {
  fetchConnectors,
  fetchConnectorYAML,
  saveConnectorYAML,
  deleteConnector,
  type ConnectorMeta,
} from '../api/client';

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
      alert('Connector konnte nicht geladen werden: ' + e);
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

    // Derive filename from `name:` field in YAML if creating new
    let targetName = editor.name;
    if (editor.isNew) {
      const match = editor.yaml.match(/^name:\s*(\S+)/m);
      targetName = match?.[1] ?? '';
      if (!targetName) {
        setSaveError('Das YAML muss ein "name:"-Feld enthalten.');
        setSaving(false);
        return;
      }
    }

    try {
      await saveConnectorYAML(targetName, editor.yaml);
      setEditor(null);
      load();
    } catch (e) {
      setSaveError(String(e));
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
      alert('Löschen fehlgeschlagen: ' + e);
    }
  };

  // Group by category
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
          <h2 className="connectors-title">Connectors</h2>
          <p className="connectors-subtitle">
            YAML-Definitionen für alle verfügbaren Monitoring-Connectors
          </p>
        </div>
        <button className="btn-primary" onClick={openNew}>+ Neuer Connector</button>
      </div>

      {loading && <p className="status-msg">Lade Connectors…</p>}

      {!loading && connectors.length === 0 && (
        <p className="status-msg">
          Keine Connectors gefunden. Lege eine YAML-Datei im connectors/-Verzeichnis an.
        </p>
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
                    <span className="connectors-row-vars">
                      {c.vars.length} Var{c.vars.length > 1 ? 's' : ''}
                    </span>
                  )}
                </div>
                <div className="connectors-row-actions">
                  <button className="btn-secondary" onClick={() => openEditor(c.name)}>
                    YAML bearbeiten
                  </button>
                  <button
                    className="btn-danger"
                    onClick={() => setConfirmDelete(c.name)}
                  >
                    Löschen
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}

      {/* YAML Editor Modal */}
      {editor && (
        <div className="yaml-overlay" onClick={e => e.target === e.currentTarget && setEditor(null)}>
          <div className="yaml-editor">
            <div className="yaml-editor-header">
              <span className="yaml-editor-title">
                {editor.isNew ? 'Neuer Connector' : `${editor.name}.yaml`}
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
              <button className="btn-secondary" onClick={() => setEditor(null)}>Abbrechen</button>
              <button className="btn-primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Speichern…' : 'Speichern'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {confirmDelete && (
        <div className="yaml-overlay" onClick={() => setConfirmDelete(null)}>
          <div className="confirm-dialog" onClick={e => e.stopPropagation()}>
            <p>Connector <strong>{confirmDelete}</strong> wirklich löschen?</p>
            <p className="confirm-hint">Diese Aktion kann nicht rückgängig gemacht werden.</p>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={() => setConfirmDelete(null)}>Abbrechen</button>
              <button className="btn-danger" onClick={() => handleDelete(confirmDelete)}>Löschen</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
