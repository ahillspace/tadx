import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { build } from 'esbuild';
import { runTests } from '@vscode/test-electron';

const extensionRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const version = process.argv.find(arg => arg.startsWith('--version='))?.slice('--version='.length) ?? 'stable';
const tempParent = resolve(tmpdir());
const root = await mkdtemp(join(tempParent, 'tadx-vscode-host-'));
const profile = join(root, 'profile');
const workspace = join(root, 'workspace');
const executable = join(root, process.platform === 'win32' ? 'tadx-host-fixture.exe' : 'tadx-host-fixture');
const testsPath = join(root, 'suite.cjs');
const child = relative(tempParent, root);
if (!child || child.startsWith('..') || isAbsolute(child) || !child.startsWith('tadx-vscode-host-')) {
  throw new Error('Refusing to use a host fixture outside its temporary parent.');
}
let passed = false;

try {
  await Promise.all([
    mkdir(join(profile, 'User'), { recursive: true }),
    mkdir(join(workspace, 'artifacts', 'workbook', 'Finance--fixture'), { recursive: true }),
    mkdir(join(workspace, 'artifacts', 'pulse-definition', 'Revenue--fixture'), { recursive: true }),
    mkdir(join(workspace, 'artifacts', 'lineage', 'workbook', 'Finance--fixture'), { recursive: true }),
    mkdir(join(root, 'calls')),
    mkdir(join(root, 'extensions')),
  ]);
  await Promise.all([
    writeFile(join(profile, 'User', 'settings.json'), JSON.stringify({
      'tadx.executablePath': executable,
      'tadx.commandTimeoutSeconds': 5,
      'security.workspace.trust.enabled': false,
      'workbench.startupEditor': 'none',
      'workbench.enableExperiments': false,
      'update.mode': 'none',
      'telemetry.telemetryLevel': 'off',
      'extensions.autoCheckUpdates': false,
      'extensions.autoUpdate': false,
      'chat.disableAIFeatures': true,
    })),
    writeFile(join(root, 'scenario.json'), '{}'),
    writeFile(join(workspace, 'artifacts', 'workbook', 'Finance--fixture', 'Finance.twb'), '<workbook/>\n'),
    writeFile(join(workspace, 'artifacts', 'workbook', 'Finance--fixture', 'metadata.json'), '{"kind":"workbook","name":"Finance local"}\n'),
    writeFile(join(workspace, 'artifacts', 'pulse-definition', 'Revenue--fixture', 'definition.json'), '{"name":"Revenue definition"}\n'),
    writeFile(join(workspace, 'artifacts', 'pulse-definition', 'Revenue--fixture', 'metadata.json'), '{"kind":"pulse-definition","name":"Revenue definition"}\n'),
    writeFile(join(workspace, 'artifacts', 'lineage', 'workbook', 'Finance--fixture', 'lineage.json'), '{"nodes":[],"edges":[]}\n'),
    writeFile(join(workspace, 'artifacts', 'lineage', 'workbook', 'Finance--fixture', 'metadata.json'), '{"kind":"lineage","name":"Finance lineage"}\n'),
    build({
      entryPoints: [join(extensionRoot, 'tests', 'host', 'index.ts')], outfile: testsPath,
      bundle: true, platform: 'node', format: 'cjs', target: 'node20', external: ['vscode'],
    }),
    promisify(execFile)('go', ['build', '-trimpath', '-o', executable, '.'], {
      cwd: join(extensionRoot, 'tests', 'fixtures', 'host'), windowsHide: true, timeout: 120_000,
    }),
  ]);
  console.log(`Running TADX extension-host tests in VS Code ${version}.`);
  await runTests({
    version,
    extensionDevelopmentPath: extensionRoot,
    extensionTestsPath: testsPath,
    extensionTestsEnv: { TADX_HOST_FIXTURE_ROOT: root },
    launchArgs: [workspace, '--user-data-dir', profile, '--extensions-dir', join(root, 'extensions'),
      '--disable-extensions', '--disable-extension=github.copilot-chat', '--disable-extension=github.copilot',
      '--skip-welcome', '--skip-release-notes', '--disable-workspace-trust', '--disable-updates'],
  });
  const report = JSON.parse(await readFile(join(root, 'report.json'), 'utf8'));
  console.log(`Host validation passed on VS Code ${report.vscodeVersion}: ${report.checks.join('; ')}.`);
  passed = true;
} finally {
  if (passed) {
    await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
  } else {
    console.error(`Host validation artifacts retained at ${root}`);
  }
}
