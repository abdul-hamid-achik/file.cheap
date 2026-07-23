import { rm } from "node:fs/promises";
import { dirname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const platformDirectory = resolve(scriptDirectory, "..");
const publicDirectory = resolve(platformDirectory, "public");
const stagedDocsDirectory = resolve(publicDirectory, "_docs");
const relativeTarget = relative(publicDirectory, stagedDocsDirectory);

if (
  !relativeTarget ||
  relativeTarget.startsWith(`..${sep}`) ||
  relativeTarget === ".."
) {
  throw new Error("refusing to clean a docs stage outside platform/public");
}

await rm(stagedDocsDirectory, { force: true, recursive: true });
