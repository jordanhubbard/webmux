// noVNC 1.7 exposes its ESM RFB implementation at the package root.
declare module '@novnc/novnc' {
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
