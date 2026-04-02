import express from 'express';
import http from 'http';
import https from 'https';
import * as fs from 'fs';
import * as path from 'path';
import cors from 'cors';
import rateLimit from 'express-rate-limit';
import { WebSocketServer } from 'ws';

import authRouter from './api/auth';
import hostsRouter from './api/hosts';
import keysRouter from './api/keys';
import sessionsRouter from './api/sessions';
import configRouter from './api/config';
import uploadRouter from './api/upload';
import aiRouter from './api/ai';
import templatesRouter from './api/templates';
import { setupWebSocket } from './websocket/handler';
import { sessionBroker } from './services/sessionBroker';
import { persistence, LOGS_DIR } from './services/persistenceManager';

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
        transport: { prefer_mosh: false, ssh_fallback: true, mosh_server_path: '' },
      },
    };
  }

  await sessionBroker.initialize();

  const app = express();

  // CORS: restrict to same-origin in secure mode, permissive in trusted mode
  if (appConfig.app.secure_mode) {
    app.use(cors({ origin: false }));
  } else {
    app.use(cors({ origin: true, credentials: true }));
  }

  app.use(express.json({ limit: '1mb' }));

  // General rate limit: 300 requests per minute per IP (applied globally)
  const apiLimiter = rateLimit({
    windowMs: 60 * 1000,
    max: 300,
    standardHeaders: true,
    legacyHeaders: false,
    message: { error: 'Too many requests, please try again later.' },
  });
  app.use(apiLimiter);

  // API routes
  app.use('/api/auth', authRouter);
  app.use('/api/hosts', hostsRouter);
  app.use('/api/keys', keysRouter);
  app.use('/api/sessions', sessionsRouter);
  app.use('/api/config', configRouter);
  app.use('/api/upload', uploadRouter);
  app.use('/api/ai', aiRouter);
  app.use('/api/sessions/templates', templatesRouter);

  // Health check
  app.get('/api/health', (_req, res) => {
    res.json({ status: 'ok', name: appConfig.app.name });
  });

  // Serve frontend static files in production
  const webDir = path.resolve(WEBMUX_ROOT, 'web');
  if (fs.existsSync(webDir)) {
    app.use(express.static(webDir));
    const indexFile = path.join(webDir, 'index.html');
    app.get('*', (_req, res) => {
      res.sendFile(indexFile);
    });
  }

  // Start HTTP server
  const httpPort = Number(process.env.HTTP_PORT) || appConfig.app.http_port;
  const httpServer = http.createServer(app);
  const wss = new WebSocketServer({ noServer: true });
  setupWebSocket(wss);

  httpServer.on('upgrade', (request, socket, head) => {
    const pathname = (request.url || '').split('?')[0];
    if (pathname.startsWith('/api/term/')) {
      wss.handleUpgrade(request, socket, head, (ws) => {
        wss.emit('connection', ws, request);
      });
    } else {
      socket.destroy();
    }
  });

  httpServer.listen(httpPort, appConfig.app.listen_host, () => {
    console.log(`WebMux HTTP server listening on ${appConfig.app.listen_host}:${httpPort}`);
  });

  // Start HTTPS server if TLS cert exists
  let httpsServer: https.Server | undefined;
  let wssSecure: WebSocketServer | undefined;
  const certFile = persistence.configPath('tls/cert.pem');
  const keyFile = persistence.configPath('tls/key.pem');
  if (fs.existsSync(certFile) && fs.existsSync(keyFile)) {
    const tlsOptions = {
      cert: fs.readFileSync(certFile),
      key: fs.readFileSync(keyFile),
    };
    const httpsPort = Number(process.env.HTTPS_PORT) || appConfig.app.https_port;
    httpsServer = https.createServer(tlsOptions, app);
    wssSecure = new WebSocketServer({ noServer: true });
    setupWebSocket(wssSecure);
    httpsServer.on('upgrade', (request, socket, head) => {
      const pathname = (request.url || '').split('?')[0];
      if (pathname.startsWith('/api/term/')) {
        wssSecure!.handleUpgrade(request, socket, head, (ws) => {
          wssSecure!.emit('connection', ws, request);
        });
      } else {
        socket.destroy();
      }
    });
    httpsServer.listen(httpsPort, appConfig.app.listen_host, () => {
      console.log(`WebMux HTTPS server listening on ${appConfig.app.listen_host}:${httpsPort}`);
    });
  }

  const shutdown = (): void => {
    console.log('Shutting down...');

    wss.clients.forEach(ws => ws.close(1001, 'Server shutting down'));
    wss.close();
    if (wssSecure) {
      wssSecure.clients.forEach(ws => ws.close(1001, 'Server shutting down'));
      wssSecure.close();
    }

    sessionBroker.shutdown();
    persistence.close();

    httpServer.close();
    httpsServer?.close();

    process.exit(0);
  };
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}

// Catch unhandled rejections globally
process.on('unhandledRejection', (reason) => {
  console.error('Unhandled rejection:', reason);
});

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
