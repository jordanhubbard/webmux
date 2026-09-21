import 'guacamole-common-js/lib/Mouse';

// Guacamole 1.5 dispatches these legacy callbacks with Mouse.Event.state.
// DefinitelyTyped covers the newer event API but omits these supported hooks.
declare module 'guacamole-common-js/lib/Mouse' {
  interface Mouse {
    onmousedown?: (state: Mouse.State) => void;
    onmouseup?: (state: Mouse.State) => void;
    onmousemove?: (state: Mouse.State) => void;
  }
}
