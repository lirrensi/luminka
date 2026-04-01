// FILE: scripts/build_icons.ts
// PURPOSE: Build canonical starter icon assets from the source PNG.
// OWNS: Windows, macOS, Linux, and Windows resource icon generation for the starter app.
// EXPORTS: none
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

import { mkdir, readFile, writeFile } from "node:fs/promises";
import * as path from "node:path";

import { Icns, IcnsImage } from "@fiahfy/icns";
import pngToIco from "png-to-ico";
import sharp from "sharp";

const root = process.cwd();
const sourcePath = path.join(root, "assets", "lumi.png");
const starterRoot = path.join(root, "starter");
const windowsDir = path.join(starterRoot, "build", "icons", "windows");
const macosDir = path.join(starterRoot, "build", "icons", "macos");
const linuxDir = path.join(starterRoot, "build", "icons", "linux");
const winresDir = path.join(starterRoot, "winres");

const windowsSizes = [16, 32, 48, 64, 128, 256, 512];
const linuxSizes = [128, 256, 512];
const icnsSizes: Array<[number, "icp4" | "icp5" | "icp6" | "ic07" | "ic08" | "ic09"]> = [
  [16, "icp4"],
  [32, "icp5"],
  [64, "icp6"],
  [128, "ic07"],
  [256, "ic08"],
  [512, "ic09"],
];

async function main(): Promise<void> {
  const source = await readFile(sourcePath);
  await Promise.all([windowsDir, macosDir, linuxDir, winresDir].map((dir) => mkdir(dir, { recursive: true })));

  const renderedWindows: string[] = [];
  let icon256Path = "";
  let icon16Path = "";
  for (const size of windowsSizes) {
    const outputPath = path.join(windowsDir, `${size}x${size}.png`);
    await writeResizedPng(source, outputPath, size);
    renderedWindows.push(outputPath);
    if (size === 256) {
      icon256Path = outputPath;
    }
    if (size === 16) {
      icon16Path = outputPath;
    }
  }

  for (const size of linuxSizes) {
    const outputPath = path.join(linuxDir, `${size}x${size}.png`);
    await writeResizedPng(source, outputPath, size);
  }

  await writeFile(path.join(winresDir, "icon.png"), await readFile(icon256Path));
  await writeFile(path.join(winresDir, "icon16.png"), await readFile(icon16Path));

  const ico = await pngToIco(renderedWindows);
  await writeFile(path.join(windowsDir, "luminka-starter.ico"), ico);

  const icns = new Icns();
  for (const [size, osType] of icnsSizes) {
    const png = await readFile(path.join(windowsDir, `${size}x${size}.png`));
    icns.append(IcnsImage.fromPNG(png, osType));
  }
  await writeFile(path.join(macosDir, "luminka-starter.icns"), icns.data);

}

async function writeResizedPng(source: Buffer, outputPath: string, size: number): Promise<void> {
  const buffer = await sharp(source)
    .resize(size, size, { fit: "contain", background: { r: 0, g: 0, b: 0, alpha: 0 } })
    .png()
    .toBuffer();
  await writeFile(outputPath, buffer);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
