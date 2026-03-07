const watchers: unknown[] = [];

export default {
  watch: (_path: string, _opts: unknown) => {
    const watcher = {
      on: (_event: string, _handler: unknown) => watcher,
      close: () => Promise.resolve(),
    };
    watchers.push(watcher);
    return watcher;
  },
};
