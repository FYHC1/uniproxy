import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
mkdirSync('bin', { recursive: true });
for (const name of ['uniproxy']) {
  const filename = join('bin', name + (process.platform === 'win32' ? '.exe' : ''));
  const result = spawnSync('go', ['build', '-trimpath', '-o', filename, './cmd/' + name], { stdio: 'inherit' });
  if (result.error) { console.error(result.error.message); process.exit(1); }
  if (result.status !== 0) process.exit(result.status ?? 1);
  console.log('Built ' + filename);
}
