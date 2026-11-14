import js from '@eslint/js'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import globals from 'globals'
import tseslint from 'typescript-eslint'

import dorgu from './eslint-rules/no-raw-colour.js'

export default tseslint.config(
  { ignores: ['dist', '../internal/webui/dist', 'node_modules', 'playwright-report', 'test-results', 'coverage'] },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      // eslint-plugin-react-hooks keeps a legacy `plugins: [...]` array on its
      // top-level configs, which ESLint 10 flat config rejects. The nested
      // `configs.flat` entry is the same rule set in flat-config shape.
      reactHooks.configs.flat['recommended-latest'],
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: { dorgu },
    rules: {
      // DESIGN.md section 9: tokens only. The stylesheet defines colours and is not a .ts or .tsx file.
      'dorgu/no-raw-colour': 'error',
      '@typescript-eslint/consistent-type-imports': 'error',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },
  {
    // The rule's own tests need colours to reject, and the contrast check is the
    // one place token values are written down as numbers to be measured.
    files: ['eslint-rules/**', 'src/design/**'],
    rules: { 'dorgu/no-raw-colour': 'off' },
  },
  {
    files: ['vite.config.ts', 'vitest.config.ts', 'playwright.config.ts', 'eslint.config.js', 'eslint-rules/**', 'scripts/**'],
    languageOptions: { globals: globals.node },
  },
)
