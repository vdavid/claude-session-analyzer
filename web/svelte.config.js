import adapter from '@sveltejs/adapter-static'
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

/** @type {import('@sveltejs/kit').Config} */
const config = {
    preprocess: vitePreprocess(),
    kit: {
        // `/session/[id]` is a client-side route over a local API, so there's nothing to prerender
        // per session. The fallback page boots the router and the page fetches its own data.
        adapter: adapter({ fallback: '200.html' }),
        paths: { relative: false },
    },
}

export default config
