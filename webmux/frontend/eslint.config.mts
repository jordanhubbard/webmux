import js from '@eslint/js';
import type { Linter } from 'eslint';
import tsPlugin from '@typescript-eslint/eslint-plugin';
import globals from 'globals';

// The plugin's public config type still describes its legacy config exports.
const recommended = tsPlugin.configs['flat/recommended'] as unknown as Linter.Config[];
const sourceFiles = ['src/**/*.{ts,tsx}'];

export default [
  { ignores: ['dist/**', 'node_modules/**', '**/*.d.ts', 'vite.config.ts'] },
  { ...js.configs.recommended, files: sourceFiles },
  ...recommended.map(config => ({ ...config, files: sourceFiles })),
  {
    files: sourceFiles,
    languageOptions: {
      globals: { ...globals.browser, ...globals.es2020 },
      parserOptions: { project: './tsconfig.json', tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      'no-console': 'off',
    },
  },
  {
    files: ['src/__tests__/**/*.{ts,tsx}', 'src/__mocks__/**/*.{ts,tsx}'],
    languageOptions: { parserOptions: { project: null } },
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-var-requires': 'off',
    },
  },
];
