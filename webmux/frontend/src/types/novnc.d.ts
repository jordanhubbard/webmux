// Upstream types describe the extensionless import and require all credential
// fields. noVNC 1.6 also exports this .js path and accepts partial credentials
// (including password-only VNC). Keep the real runtime behavior unchanged.
declare module '@novnc/novnc/lib/rfb.js' {
  import RFB from '@novnc/novnc/lib/rfb';

  type Credentials = Partial<Parameters<RFB['sendCredentials']>[0]>;
  type Options = Omit<NonNullable<ConstructorParameters<typeof RFB>[2]>, 'credentials'> & {
    credentials?: Credentials;
  };

  export default class VncClient extends RFB {
    constructor(target: Element, url: string | WebSocket | RTCDataChannel, options?: Options);
    sendCredentials(credentials: Credentials): void;
  }
}
