export interface Translations {
  app: {
    tagline: string;
    nav: { dashboard: string; connectors: string };
    updates: (n: number) => string;
    hosts: (n: number) => string;
    refresh: string;
    refreshing: string;
    addHost: string;
  };
  dashboard: {
    loading: string;
    error: (e: string) => string;
    noHosts: string;
    noHostsHint: string;
    noChecks: string;
  };
  checkRow: {
    pending: string;
    running: string;
    updated: string;
    failed: string;
    update: string;
    changelog: string;
    updateError: (e: string) => string;
  };
  wizard: {
    title: string;
    hostNameLabel: string;
    hostNamePlaceholder: string;
    detected: (label: string) => string;
    connectorsLabel: string;
    search: string;
    recommended: string;
    runOnHost: (name: string) => string;
    installSubtitle: string;
    copy: string;
    copied: string;
    agentHint: string;
    tipTitle: string;
    tipText: string;
    back: string;
    generate: string;
    done: string;
  };
  hostDetail: {
    back: string;
    agentOnline: string;
    agentStale: string;
    agentOffline: string;
    lastSeen: (s: string) => string;
    ignore: string;
    unignore: string;
    ignoredLabel: string;
    commandHistory: string;
    noCommands: string;
    showOutput: string;
    hideOutput: string;
    removeConnector: string;
    confirmRemoveConnector: (name: string) => string;
    deleteHost: string;
    confirmDeleteHost: (name: string) => string;
  };
  connectors: {
    title: string;
    subtitle: string;
    newConnector: string;
    loading: string;
    empty: string;
    vars: (n: number) => string;
    editYaml: string;
    delete: string;
    newConnectorTitle: string;
    saving: string;
    save: string;
    cancel: string;
    noNameError: string;
    loadError: (e: string) => string;
    saveError: (e: string) => string;
    deleteError: (e: string) => string;
    confirmDelete: (name: string) => string;
    confirmDeleteHint: string;
  };
}
