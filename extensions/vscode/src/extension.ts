import * as vscode from 'vscode';
import { Controller } from './controller.js';

export function activate(context: vscode.ExtensionContext): { ready: Promise<void>; getSnapshot: Controller['getSnapshot']; getCompatibility: Controller['getCompatibility'] } | undefined {
  if (!vscode.workspace.isTrusted) return undefined;
  const controller = new Controller(context);
  context.subscriptions.push(controller);
  return { ready: controller.ready, getSnapshot: () => controller.getSnapshot(), getCompatibility: () => controller.getCompatibility() };
}
