import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// noVNC's lib/util/browser.js uses a top-level `await` export that Rollup's
// CJS transformer cannot parse. Patch it out at build time — the affected
// symbol (supportsWebCodecsH264Decode) is an optional performance hint for
// H.264 hardware decode; setting it to false is safe for all VNC sessions.
const novncPatch = {
  name: 'novnc-tla-patch',
  transform(code: string, id: string) {
    if (id.includes('@novnc') && id.endsWith('browser.js')) {
      return code.replace(
        /exports\.supportsWebCodecsH264Decode\s*=\s*\S+\s*=\s*await[^;]+;/,
        'exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = false;',
      );
    }
  },
};

export default defineConfig({
  plugins: [react(), novncPatch],
  resolve: {
    alias: {
      '@frontend': path.resolve(__dirname, 'src'),
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
        changeOrigin: true,
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
    server: {
      deps: {
        // Run @novnc/novnc through Vite's transform pipeline so the
        // novncPatch plugin can remove the top-level await before Node loads it.
        inline: ['@novnc/novnc'],
      },
    },
  },
});
