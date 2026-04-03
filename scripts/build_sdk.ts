// FILE: scripts/build_sdk.ts
// PURPOSE: Transpile the in-repo Luminka TypeScript SDK into browser-ready dist outputs for app embeds and external consumption.
// OWNS: SDK transpilation and emission for sdk/dist plus starter and example build targets.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_phase3_surface_examples_2026-03-30.md

import { mkdir, readFile, writeFile } from "node:fs/promises";
import * as path from "node:path";
import * as ts from "typescript";

const root = process.cwd();
const sourcePath = path.join(root, "luminka", "sdk", "luminka.ts");
const outputPaths = [
  path.join(root, "sdk", "dist", "luminka.js"),
  path.join(root, "starter", "dist", "luminka.js"),
  path.join(root, "examples", "hello", "dist", "luminka.js"),
  path.join(root, "examples", "kanban", "dist", "luminka.js"),
];

async function main(): Promise<void> {
  const source = await readFile(sourcePath, "utf8");
  const result = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
      removeComments: false,
      sourceMap: false,
    },
    fileName: sourcePath,
  });

  for (const outputPath of outputPaths) {
    await mkdir(path.dirname(outputPath), { recursive: true });
    await writeFile(outputPath, result.outputText, "utf8");
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
