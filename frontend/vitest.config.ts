import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    // app.css is read as text by app.css.test.ts; vitest returns an empty string for CSS otherwise.
    css: { include: [/app\.css/] }
  }
})
