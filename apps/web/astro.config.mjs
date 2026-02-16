// @ts-check
import { defineConfig } from 'astro/config';

import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
  site: 'https://riebschlager.github.io',
  base: '/mp3-collection',
  output: 'static',
  vite: {
    plugins: [tailwindcss()]
  }
});