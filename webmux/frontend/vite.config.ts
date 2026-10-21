import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'path';

const config = {
  plugins: [react()],
  resolve: {
    alias: {
      '@frontend': path.resolve(import.meta.dirname, 'src'),
      'guacamole-common-js': path.resolve(import.meta.dirname, '../node_modules/guacamole-common-js/dist/esm/guacamole-common.js'),
      '@testing-library/react': path.resolve(import.meta.dirname, '../node_modules/@testing-library/react'),
      react: path.resolve(import.meta.dirname, '../node_modules/react'),
      'react-dom': path.resolve(import.meta.dirname, '../node_modules/react-dom'),
    },
    preserveSymlinks: true,
  },
  server: {
    fs: {
      allow: [path.resolve(import.meta.dirname, '../..')],
    },
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        // Preserve the browser-facing Host for the server's WebSocket origin check.
        changeOrigin: false,
        ws: true,
      },
    },
  },
  optimizeDeps: {
    include: ['@novnc/novnc'],
  },
  build: {
    target: 'esnext',
    outDir: '../web',
    emptyOutDir: true,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: 'xterm', test: /[\\/]node_modules[\\/]@xterm[\\/]/ },
            { name: 'novnc', test: /[\\/]node_modules[\\/]@novnc[\\/]/ },
          ],
        },
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    include: ['../../tests/frontend/**/*.{test,spec}.?(c|m)[jt]s?(x)'],
    setupFiles: './src/test/setup.ts',
  },
};

export default defineConfig(config);
