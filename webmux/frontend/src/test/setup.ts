import '@testing-library/jest-dom';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// jsdom has no native dialog lifecycle. Browser tests cover focus and inertness.
HTMLDialogElement.prototype.showModal = function () {
  this.setAttribute('open', '');
};
HTMLDialogElement.prototype.close = function () {
  this.removeAttribute('open');
};

afterEach(() => {
  cleanup();
});

// Ensure localStorage is available in jsdom test environment
if (typeof globalThis.localStorage === 'undefined' || typeof globalThis.localStorage.getItem !== 'function') {
  const store = new Map<string, string>();
  const localStorage: Storage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, String(value)),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() { return store.size; },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: localStorage,
  });
}
