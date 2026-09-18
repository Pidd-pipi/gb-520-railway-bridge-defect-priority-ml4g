import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// In production Nginx proxies /api to the backend; mirror that in dev so the
// same relative request() paths work in both environments.
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: process.env.BACKEND_URL || 'http://127.0.0.1:19520',
        changeOrigin: true,
      },
    },
  },
});
