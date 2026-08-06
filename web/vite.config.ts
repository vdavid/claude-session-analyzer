import { sveltekit } from '@sveltejs/kit/vite'
import tailwindcss from '@tailwindcss/vite'
import { loadEnv } from 'vite'
import { defineConfig } from 'vitest/config'

// Ports come from the repo root's committed `.env`, the same file the Go server reads, so the two
// can't drift. `envDir` points Vite at it; the `CSA_` prefix opts those names into `loadEnv`.
const env = loadEnv('development', '..', 'CSA_')
const backendPort = Number(env.CSA_BACKEND_PORT)
const frontendPort = Number(env.CSA_FRONTEND_PORT)

if (!backendPort || !frontendPort) {
    throw new Error('Set CSA_BACKEND_PORT and CSA_FRONTEND_PORT in the repo root `.env`.')
}

export default defineConfig({
    plugins: [tailwindcss(), sveltekit()],
    envDir: '..',
    server: {
        host: '127.0.0.1',
        port: frontendPort,
        strictPort: true,
        // The API is served same-origin through this proxy, so the browser sends no preflight and
        // the app needs no base URL. The Go server's own CORS allowlist still guards direct calls.
        proxy: {
            '/api': {
                target: `http://127.0.0.1:${backendPort}`,
                changeOrigin: false,
            },
        },
    },
    preview: { host: '127.0.0.1', port: frontendPort, strictPort: true },
    test: { include: ['tests/**/*.test.ts'] },
})
