const path = require('path');

const backendSrc = path.resolve(__dirname, 'src');
const testsDir = path.resolve(__dirname, '../../tests/backend');

module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: [testsDir],
  testMatch: ['**/*.test.ts'],
  // Test suites share filesystem state via the `persistence` module-level singleton
  // and per-test process.env.WEBMUX_HOME swaps. Running in a single worker avoids
  // cross-suite races; a proper fix would inject persistence rather than singleton.
  maxWorkers: 1,
  testPathIgnorePatterns: ['/node_modules/', '/dist/'],
  modulePathIgnorePatterns: ['<rootDir>/dist/'],
  modulePaths: [path.resolve(__dirname, 'node_modules'), path.resolve(__dirname, '../node_modules')],
  moduleNameMapper: {
    '^argon2$': path.resolve(backendSrc, '__mocks__/argon2.ts'),
    '^node-pty$': path.resolve(backendSrc, '__mocks__/node-pty.ts'),
    '^chokidar$': path.resolve(backendSrc, '__mocks__/chokidar.ts'),
    '^@backend/(.*)$': path.resolve(backendSrc, '$1'),
  },
  transform: {
    '^.+\\.ts$': ['ts-jest', {
      tsconfig: path.resolve(testsDir, 'tsconfig.json'),
    }],
  },
};
