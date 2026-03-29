module.exports = {
  root: true,
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 2022,
    sourceType: 'module',
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
    '@typescript-eslint/no-var-requires': 'warn',
    'no-console': 'off',
    'no-useless-escape': 'warn',
  },
  env: {
    node: true,
    es2022: true,
  },
  ignorePatterns: ['dist/', 'node_modules/', '**/*.js', '**/*.d.ts'],
  overrides: [
    {
      // __mocks__ files are not in tsconfig, skip typed linting for them
      files: ['src/__mocks__/**/*.ts'],
      parserOptions: {
        project: null,
      },
      rules: {
        '@typescript-eslint/no-var-requires': 'off',
      },
    },
  ],
};
