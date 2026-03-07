module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  testMatch: ['**/__tests__/**/*.test.ts'],
  testPathIgnorePatterns: ['/node_modules/', '/dist/'],
  modulePathIgnorePatterns: ['<rootDir>/dist/'],
  moduleNameMapper: {
    '^argon2$': '<rootDir>/src/__mocks__/argon2.ts',
    '^node-pty$': '<rootDir>/src/__mocks__/node-pty.ts',
    '^chokidar$': '<rootDir>/src/__mocks__/chokidar.ts',
  },
};
