// Build an explicit public allowlist, never a copy of the checkout or docs tree.
import { copyFile, lstat, mkdir, readdir, realpath, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export const publicFiles = Object.freeze([
  ['site/index.html', 'index.html'],
  ['docs/reference/capability-map.html', 'capabilities.html'],
  ['docs/reference/capabilities.json', 'capabilities.json'],
  ['scripts/install.sh', 'install.sh'],
  ['scripts/install.ps1', 'install.ps1'],
]);
const repo = dirname(dirname(fileURLToPath(import.meta.url)));

export async function buildSite(root = repo, destination = join(root, '_site')) {
  root = await realpath(root);
  destination = resolve(destination);
  // Refuse an existing/nonempty destination instead of deleting unknown files or
  // silently shipping stale files from an earlier build.
  try {
    const existing = await lstat(destination);
    if (!existing.isDirectory() || existing.isSymbolicLink() || (await readdir(destination)).length) {
      throw new Error('Site output must be a new or empty real directory.');
    }
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  for (const [source] of publicFiles) {
    const sourcePath = join(root, source);
    const info = await lstat(sourcePath);
    if (!info.isFile() || info.isSymbolicLink() || await realpath(sourcePath) !== sourcePath) {
      throw new Error(`Public input must be a regular file without symlinked parents: ${source}`);
    }
  }
  await mkdir(destination, { recursive: true });
  for (const [source, target] of publicFiles) {
    await copyFile(join(root, source), join(destination, target));
  }
  await writeFile(join(destination, '.nojekyll'), '');
  return destination;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  if (process.argv.length > 3) throw new Error('Usage: node scripts/build-site.mjs [empty-output-directory]');
  console.log(await buildSite(repo, process.argv[2] || join(repo, '_site')));
}
