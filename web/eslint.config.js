import eslint from '@eslint/js'
import svelte from 'eslint-plugin-svelte'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
    eslint.configs.recommended,
    ...tseslint.configs.strict,
    ...svelte.configs.recommended,
    {
        languageOptions: {
            globals: { ...globals.browser, ...globals.node },
        },
    },
    {
        files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
        languageOptions: {
            parserOptions: { parser: tseslint.parser },
        },
    },
    {
        rules: {
            '@typescript-eslint/no-explicit-any': 'error',
        },
    },
    {
        ignores: ['build/', '.svelte-kit/', 'dist/', 'node_modules/'],
    },
)
