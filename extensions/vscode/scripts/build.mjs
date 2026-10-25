import { build, context } from 'esbuild';
import { copyFile, mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

// Ship the maintained installers so setup uses the same download and replacement guards as the CLI.
await mkdir(new URL('../dist/installers/', import.meta.url), { recursive: true });
for (const name of ['install.sh', 'install.ps1']) {
  await copyFile(fileURLToPath(new URL(`../../../scripts/${name}`, import.meta.url)),
    fileURLToPath(new URL(`../dist/installers/${name}`, import.meta.url)));
}

const options = {
  entryPoints: ['src/extension.ts'],
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node20',
  external: ['vscode'],
  outfile: 'dist/extension.js',
  sourcemap: false,
  logLevel: 'info',
};
if (process.argv.includes('--watch')) {
  await (await context(options)).watch();
} else {
  await build(options);
}
