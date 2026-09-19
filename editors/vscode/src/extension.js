const path = require("node:path");
const fs = require("node:fs");
const vscode = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

// Store tests and fga.mod keep the yaml language id so every YAML extension
// still works on them; the server is selected by file name instead.
const selector = [
  { scheme: "file", language: "fga" },
  { scheme: "file", pattern: "**/*.fga.yaml" },
  { scheme: "file", pattern: "**/*.fga.yml" },
  { scheme: "file", pattern: "**/fga.mod" },
];

function resolveServer(config) {
  const configured = config.get("server.path") || "fga-lsp";

  if (path.isAbsolute(configured)) {
    return configured;
  }

  for (const folder of vscode.workspace.workspaceFolders ?? []) {
    const candidate = path.join(folder.uri.fsPath, configured);
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }

  return configured;
}

async function activate(context) {
  const config = vscode.workspace.getConfiguration("fga");
  const command = resolveServer(config);

  const server = {
    command,
    args: config.get("server.arguments") ?? [],
    transport: TransportKind.stdio,
  };

  client = new LanguageClient("fga", "OpenFGA", { run: server, debug: server }, {
    documentSelector: selector,
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/{*.fga,*.fga.yaml,*.fga.yml,fga.mod}"),
    },
  });

  try {
    await client.start();
  } catch (error) {
    vscode.window.showErrorMessage(
      `OpenFGA: could not start ${command}. Build it with \`make\` or set fga.server.path. (${error.message})`,
    );
    client = undefined;
    return;
  }

  context.subscriptions.push({ dispose: () => client?.stop() });
}

function deactivate() {
  return client?.stop();
}

module.exports = { activate, deactivate };
