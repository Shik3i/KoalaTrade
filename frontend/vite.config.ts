import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

const apiTarget = process.env.KOALA_API_TARGET ?? 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': apiTarget,
      '/healthz': apiTarget
    }
  }
});
