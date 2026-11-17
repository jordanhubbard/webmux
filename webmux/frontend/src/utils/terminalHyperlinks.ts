import type { IDisposable, Terminal } from '@xterm/xterm';

type Marker = { isDisposed: boolean };
type Entry = { id: number; lines: Marker[] };
type LinkService = {
  _dataByLinkId: Map<number, Entry>;
  _removeMarkerFromLink(entry: Entry, marker: Marker): void;
};
type LinkCore = {
  _oscLinkService: LinkService;
  _bufferService: { buffers: { normal: { clearAllMarkers(): void }; alt: { clearAllMarkers(): void } } };
  _inputHandler: { _curAttrData: { extended: { urlId: number } } };
};

/** Keep the active OSC 8 target alive when its last buffer row is disposed.
 * xterm 6 stores that pen outside the buffers, but its link service counts only
 * row markers. A buffer switch can otherwise leave a dangling pen reference.
 * This adapter is version-qualified by the actual renderer/ConPTY controls.
 */
export function retainActiveTerminalHyperlink(term: Terminal): IDisposable {
  const core = (term as unknown as { _core: LinkCore })._core;
  const service = core?._oscLinkService;
  if (!service || !(service._dataByLinkId instanceof Map) || typeof service._removeMarkerFromLink !== 'function' || !core._inputHandler?._curAttrData?.extended || typeof core._bufferService?.buffers?.normal?.clearAllMarkers !== 'function' || typeof core._bufferService?.buffers?.alt?.clearAllMarkers !== 'function') {
    throw new Error('This terminal renderer does not support the WebMux hyperlink profile');
  }
  const remove = service._removeMarkerFromLink;
  const currentId = () => core._inputHandler._curAttrData.extended.urlId;
  let held: { entry: Entry; marker: Marker } | undefined;
  const release = () => {
    const value = held; held = undefined;
    if (value && value.entry.lines.length === 0 && service._dataByLinkId.get(value.entry.id) === value.entry) {
      // Let xterm perform its own map/key cleanup. This marker has already
      // been disposed; it is retained only until the pen changes or is used.
      value.entry.lines.push(value.marker);
      remove.call(service, value.entry, value.marker);
    }
  };
  const guardedRemove = function (this: LinkService, entry: Entry, marker: Marker) {
    if (entry.id === currentId() && entry.lines.length === 1 && entry.lines[0] === marker) {
      release();
      entry.lines.pop();
      held = { entry, marker };
      return;
    }
    remove.call(this, entry, marker);
  };
  service._removeMarkerFromLink = guardedRemove;
  const changesPen = term.parser.registerOscHandler(8, data => {
    const separator = data.indexOf(';');
    if (separator !== -1 && (data.slice(separator + 1) !== '' || data.slice(0, separator).trim() === '')) release();
    return false; // xterm still executes its normal OSC 8 handler.
  });
  const clearForReset = () => {
    release();
    // xterm 6 replaces its buffers on reset without disposing their markers.
    // Dispose them while their link entries are reachable, then drop the pen.
    core._bufferService.buffers.normal.clearAllMarkers();
    core._bufferService.buffers.alt.clearAllMarkers();
    release();
  };
  const reset = term.parser.registerEscHandler({ final: 'c' }, () => { clearForReset(); return false; });
  const originalReset = term.reset;
  const resetTerminal = function (this: Terminal) { clearForReset(); originalReset.call(this); };
  term.reset = resetTerminal;
  const parsed = term.onWriteParsed(() => {
    if (!held) return;
    if (held.entry.id !== currentId()) release();
    else if (held.entry.lines.some(marker => !marker.isDisposed)) held = undefined;
  });
  return { dispose() {
    changesPen.dispose(); reset.dispose(); parsed.dispose(); release();
    if (term.reset === resetTerminal) term.reset = originalReset;
    if (service._removeMarkerFromLink === guardedRemove) service._removeMarkerFromLink = remove;
  } };
}
