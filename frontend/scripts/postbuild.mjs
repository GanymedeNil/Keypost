// `output: 'standalone'` does not copy static assets next to the standalone server.
// Do it after every build so `pnpm start` (node .next/standalone/server.js) serves a complete app.
import { cpSync, existsSync } from 'node:fs';

const standalone = '.next/standalone';
if (!existsSync(standalone)) {
  console.error('postbuild: .next/standalone not found (is output: "standalone" set?)');
  process.exit(1);
}
cpSync('.next/static', `${standalone}/.next/static`, { recursive: true });
if (existsSync('public')) cpSync('public', `${standalone}/public`, { recursive: true });
console.log('postbuild: copied static assets into .next/standalone');
