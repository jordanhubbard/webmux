import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'path';

const config = {
  plugins: [react()],
  resolve: {
    alias: {
      '@frontend': path.resolve(__dirname, 'src'),
      'guacamole-common-js': path.resolve(__dirname, '../node_modules/guacamole-common-js/dist/esm/guacamole-common.js'),
      '@testing-library/react': path.resolve(__dirname, '../node_modules/@testing-library/react'),
      react: path.resolve(__dirname, '../node_modules/react'),
      'react-dom': path.resolve(__dirname, '../node_modules/react-dom'),
    },
    preserveSymlinks: true,
  },
  server: {
    fs: {
      allow: [path.resolve(__dirname, '../..')],
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
    rollupOptions: {
      output: {
        manualChunks: {
          xterm: ['@xterm/xterm', '@xterm/addon-fit', '@xterm/addon-web-links'],
          novnc: ['@novnc/novnc'],
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
