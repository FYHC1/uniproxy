import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({ root: 'src/web', plugins: [react()], build: { outDir: '../../internal/webui/assets/dist', emptyOutDir: true }, server: { port: 5173, strictPort: true, proxy: { '/api': 'http://127.0.0.1:9090' } } });
