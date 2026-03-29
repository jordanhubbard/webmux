module.exports = {
  root: true,
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
    ecmaFeatures: { jsx: true },
    project: './tsconfig.json',
    tsconfigRootDir: __dirname,
  },
  plugins: ['@typescript-eslint'],
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
  ],
  rules: {
    '@typescript-eslint/no-explicit-any': 'warn',
    '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
    'no-console': 'off',
  },
  env: {
    browser: true,
    es2020: true,
  },
  ignorePatterns: ['dist/', 'node_modules/', '*.js', '*.d.ts', 'vite.config.ts'],
  overrides: [
    {
      // __tests__ and __mocks__ are excluded from tsconfig — skip typed linting
      files: ['src/__tests__/**/*.{ts,tsx}', 'src/__mocks__/**/*.{ts,tsx}'],
      parserOptions: {
        project: null,
      },
      rules: {
        '@typescript-eslint/no-explicit-any': 'off',
        '@typescript-eslint/no-var-requires': 'off',
      },
    },
  ],
};
