import express from 'express';
import http from 'http';
import https from 'https';
import * as fs from 'fs';
import * as path from 'path';
import cors from 'cors';
import { WebSocketServer } from 'ws';

import authRouter from './api/auth';
import hostsRouter from './api/hosts';
import sessionsRouter from './api/sessions';
import configRouter from './api/config';
import { setupWebSocket } from './websocket/handler';
import { sessionBroker } from './services/sessionBroker';
import { persistence } from './services/persistenceManager';

const WEBMUX_ROOT = process.env.WEBMUX_ROOT || path.join(__dirname, '../..');

async function main(): Promise<void> {
  let appConfig;
  try {
    appConfig = persistence.loadApp();
  } catch {
    console.warn('Could not load app.yaml, using defaults');
    appConfig = {
      app: {
        name: 'webmux',
        listen_host: '0.0.0.0',
        http_port: 8080,
        https_port: 8443,
        secure_mode: false,
        trusted_http_allowed: true,
        default_term: { cols: 80, rows: 24, font_size: 14 },
        transport: { prefer_mosh: false, ssh_fallback: true },
      },
    };
  }

  await sessionBroker.initialize();

  const app = express();
  app.use(cors({ origin: true, credentials: true }));
  app.use(express.json());
  app.use(express.urlencoded({ extended: true }));

  // API routes
  app.use('/api/auth', authRouter);
  app.use('/api/hosts', hostsRouter);
  app.use('/api/sessions', sessionsRouter);
  app.use('/api/config', configRouter);

  // Health check
  app.get('/api/health', (_req, res) => {
    res.json({ status: 'ok', name: appConfig.app.name });
  });

  // Serve frontend static files in production
  const webDir = path.join(WEBMUX_ROOT, 'web');
  if (fs.existsSync(webDir)) {
    app.use(express.static(webDir));
    app.get('*', (_req, res) => {
      res.sendFile(path.join(webDir, 'index.html'));
    });
  }

  // Start HTTP server
  const httpPort = Number(process.env.HTTP_PORT) || appConfig.app.http_port;
  const httpServer = http.createServer(app);
  const wss = new WebSocketServer({ server: httpServer, path: '/api/term' });
  setupWebSocket(wss);

  httpServer.listen(httpPort, appConfig.app.listen_host, () => {
    console.log(`WebMux HTTP server listening on ${appConfig.app.listen_host}:${httpPort}`);
  });

  // Start HTTPS server if TLS cert exists
  const certFile = path.join(WEBMUX_ROOT, 'config/tls/cert.pem');
  const keyFile = path.join(WEBMUX_ROOT, 'config/tls/key.pem');
  if (fs.existsSync(certFile) && fs.existsSync(keyFile)) {
    const tlsOptions = {
      cert: fs.readFileSync(certFile),
      key: fs.readFileSync(keyFile),
    };
    const httpsPort = Number(process.env.HTTPS_PORT) || appConfig.app.https_port;
    const httpsServer = https.createServer(tlsOptions, app);
    const wssSecure = new WebSocketServer({ server: httpsServer, path: '/api/term' });
    setupWebSocket(wssSecure);
    httpsServer.listen(httpsPort, appConfig.app.listen_host, () => {
      console.log(`WebMux HTTPS server listening on ${appConfig.app.listen_host}:${httpsPort}`);
    });
  }

  // Graceful shutdown
  const shutdown = (): void => {
    console.log('Shutting down...');
    persistence.close();
    process.exit(0);
  };
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
