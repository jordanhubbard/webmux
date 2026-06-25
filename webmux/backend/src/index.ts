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
import uploadRouter, { startPurgeTimer, stopPurgeTimer } from './api/upload';
import aiRouter from './api/ai';
import templatesRouter from './api/templates';
import vncRouter from './api/vnc';
import rdpRouter from './api/rdp';
import agentsRouter from './api/agents';
import { setupWebSocket } from './websocket/handler';
import { setupVncWebSocket } from './websocket/vncHandler';
import { setupRdpWebSocket } from './websocket/rdpHandler';
import { sessionBroker } from './services/sessionBroker';
import { vncBroker } from './services/vncBroker';
import { rdpBroker } from './services/rdpBroker';
import { persistence } from './services/persistenceManager';
import { normalizeAppConfig } from './services/appConfig';

const WEBMUX_ROOT = process.env.WEBMUX_ROOT || path.join(__dirname, '../..');

async function main(): Promise<void> {
  let appConfig;
  try {
    appConfig = normalizeAppConfig(persistence.loadApp());
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
        terminal_grid: { max_cols: null, max_rows: null },
        ui: { default_pane: 'terminals', host_switcher: { enabled: false, suffixes: [], hosts: [] } },
        agents: { enabled: false, combined_pane: true, disable_in_multi_user_mode: true, definitions: [] },
        transport: { prefer_mosh: false, ssh_fallback: true, mosh_server_path: '' },
      },
    };
  }

  await sessionBroker.initialize();
  await vncBroker.initialize();
  await rdpBroker.initialize();
  const enforceAgentAccessPolicy = (): void => {
    sessionBroker.enforceAgentAccessPolicy().catch(err => {
      console.error('Agent access policy enforcement failed:', err);
    });
  };
  persistence.watchConfig('app.yaml', enforceAgentAccessPolicy);
  persistence.watchConfig('auth.yaml', enforceAgentAccessPolicy);
  startPurgeTimer();

  // Slave mode: auto-create an exec session on startup if WEBMUX_SLAVE_HOST is set.
  // Used by agentOS to connect to the local agent console with no user interaction.
  // In slave mode we own the session list entirely — clear any saved state first.
  const slaveHost = process.env.WEBMUX_SLAVE_HOST;
  if (slaveHost) {
    for (const s of sessionBroker.list()) {
      await sessionBroker.delete(s.id);
    }
    const slavePort = process.env.WEBMUX_SLAVE_PORT ? Number(process.env.WEBMUX_SLAVE_PORT) : 0;
    try {
      await sessionBroker.create({
        hostname: slaveHost,
        port: slavePort,
        username: 'console',
        transport: 'exec',
        row: 0,
        col: 0,
      }, 'system');
      console.log(`Slave mode: auto-connected to ${slaveHost}:${slavePort}`);
    } catch (err) {
      console.error('Slave mode: failed to create exec session:', (err as Error).message);
    }
  }

  const app = express();

  // CORS: restrict to same-origin in secure mode, permissive in trusted mode
  if (appConfig.app.secure_mode) {
    app.use(cors({ origin: false }));
  } else {
    app.use(cors({ origin: true, credentials: true }));
  }

  app.use(express.json({ limit: '1mb' }));

  // General rate limit: 300 requests per minute per IP (applied globally,
  // except /api/health which monitoring may poll frequently).
  const apiLimiter = rateLimit({
    windowMs: 60 * 1000,
    max: 300,
    standardHeaders: true,
    legacyHeaders: false,
    message: { error: 'Too many requests, please try again later.' },
    skip: req => req.path === '/api/health',
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
  app.use('/api/vnc', vncRouter);
  app.use('/api/rdp', rdpRouter);
  app.use('/api/agents', agentsRouter);

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
  const wssVnc = new WebSocketServer({ noServer: true });
  setupVncWebSocket(wssVnc);
  const wssRdp = new WebSocketServer({ noServer: true });
  setupRdpWebSocket(wssRdp);

  httpServer.on('upgrade', (request, socket, head) => {
    const pathname = (request.url || '').split('?')[0];
    if (pathname.startsWith('/api/term/')) {
      wss.handleUpgrade(request, socket, head, (ws) => {
        wss.emit('connection', ws, request);
      });
    } else if (pathname.startsWith('/api/vnc/ws/')) {
      wssVnc.handleUpgrade(request, socket, head, (ws) => {
        wssVnc.emit('connection', ws, request);
      });
    } else if (pathname.startsWith('/api/rdp/ws/')) {
      wssRdp.handleUpgrade(request, socket, head, (ws) => {
        wssRdp.emit('connection', ws, request);
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
      } else if (pathname.startsWith('/api/vnc/ws/')) {
        wssVnc.handleUpgrade(request, socket, head, (ws) => {
          wssVnc.emit('connection', ws, request);
        });
      } else if (pathname.startsWith('/api/rdp/ws/')) {
        wssRdp.handleUpgrade(request, socket, head, (ws) => {
          wssRdp.emit('connection', ws, request);
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
    wssVnc.clients.forEach(ws => ws.close(1001, 'Server shutting down'));
    wssVnc.close();
    wssRdp.clients.forEach(ws => ws.close(1001, 'Server shutting down'));
    wssRdp.close();
    if (wssSecure) {
      wssSecure.clients.forEach(ws => ws.close(1001, 'Server shutting down'));
      wssSecure.close();
    }

    sessionBroker.shutdown();
    vncBroker.shutdown();
    rdpBroker.shutdown();
    stopPurgeTimer();
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
