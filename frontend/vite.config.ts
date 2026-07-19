import react from '@vitejs/plugin-react'
import { configDefaults, defineConfig } from 'vitest/config'

export default defineConfig(({ command }) => ({
  plugins: [
    react(),
    {
      name: 'production-content-security-policy',
      transformIndexHtml(html) {
        if (command !== 'build') return html
        return html
          .replace("script-src 'self' 'unsafe-inline'", "script-src 'self'")
          .replace(
            "connect-src 'self' ws://localhost:* http://localhost:*",
            "connect-src 'self'",
          )
      },
    },
  ],
  build: {
    target: 'es2022',
    sourcemap: false,
    outDir: '../internal/frontendassets/bundle/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    exclude: [...configDefaults.exclude, 'e2e/**'],
    restoreMocks: true,
  },
}))
