import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// API_TARGET is set by docker-compose so the vite dev server inside Docker
// can forward /api requests to the server container.
const apiTarget = process.env.API_TARGET ?? 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 4000,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
});
