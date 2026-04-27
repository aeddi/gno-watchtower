import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

// Dev mode proxies API calls to a locally running scribe binary on :8090.
// Production builds emit static assets that scribe embeds via go:embed.
export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: { '@': path.resolve(__dirname, 'src') },
    },
    server: {
        port: 5173,
        proxy: {
            '/api': 'http://localhost:8090',
            '/docs/rules': 'http://localhost:8090',
            '/docs/handlers': 'http://localhost:8090',
            '/metrics': 'http://localhost:8090',
            '/health': 'http://localhost:8090',
        },
    },
    build: {
        outDir: 'dist',
        emptyOutDir: true,
        sourcemap: true,
    },
    test: {
        environment: 'jsdom',
        globals: true,
    },
})
