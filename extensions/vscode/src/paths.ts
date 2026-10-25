import { realpath } from 'node:fs/promises';
import path from 'node:path';

export function relativeArtifactPath(value: string): string {
  if (!value || value.includes(String.fromCharCode(0)) || /[\\:]/u.test(value) || value.startsWith('/') || value.split('/').some(part => !part || part === '.' || part === '..')) {
    throw new Error('The artifact path must stay inside its registered workspace.');
  }
  return value;
}

/** Resolve the actual file before opening it, including symlinks and Windows junctions. */
export async function workspaceFile(root: string | undefined, relative: string | undefined): Promise<string> {
  if (!root || !relative || !path.isAbsolute(root)) throw new Error('The workspace or artifact path is unavailable. Reload its inventory.');
  relativeArtifactPath(relative);
  const [actualRoot, actualFile] = await Promise.all([realpath(root), realpath(path.join(root, ...relative.split('/')))]);
  const difference = path.relative(actualRoot, actualFile);
  if (!difference || difference === '..' || difference.startsWith(`..${path.sep}`) || path.isAbsolute(difference)) {
    throw new Error('The artifact resolves outside its registered workspace.');
  }
  return actualFile;
}

export function tableauUrl(webpageUrl: string | undefined, serverUrl: string | undefined): URL {
  if (!webpageUrl || !serverUrl) throw new Error('TADX does not expose a browser URL for this artifact.');
  const url = new URL(webpageUrl);
  const server = new URL(serverUrl);
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.origin !== server.origin) {
    throw new Error('The artifact browser URL does not belong to the selected Tableau server.');
  }
  return url;
}
