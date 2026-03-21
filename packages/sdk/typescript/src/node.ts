import { readFile, readdir, stat } from "node:fs/promises";
import { basename, join, relative } from "node:path";
import type { BoxerClient } from "./client.js";

/**
 * Upload a local file or directory to the Boxer file store.
 *
 * If `localPath` is a directory, all files inside it are uploaded recursively,
 * preserving the directory structure under `remotePath` (defaults to the
 * directory name).
 *
 * Returns the list of remote paths that were uploaded.
 *
 * Note: Requires a runtime with Node.js-compatible `fs` APIs (Node, Bun, Deno).
 */
export async function uploadPath(
  client: BoxerClient,
  localPath: string,
  remotePath?: string,
): Promise<string[]> {
  const info = await stat(localPath);

  if (info.isDirectory()) {
    const prefix = remotePath ?? basename(localPath);
    const uploaded: string[] = [];
    await uploadDir(client, localPath, localPath, prefix, uploaded);
    return uploaded.sort();
  }

  const dest = remotePath ?? basename(localPath);
  const content = await readFile(localPath);
  await client.uploadFile(dest, content);
  return [dest];
}

async function uploadDir(
  client: BoxerClient,
  rootDir: string,
  currentDir: string,
  prefix: string,
  uploaded: string[],
): Promise<void> {
  const entries = await readdir(currentDir, { withFileTypes: true });

  for (const entry of entries) {
    const fullPath = join(currentDir, entry.name);
    if (entry.isDirectory()) {
      await uploadDir(client, rootDir, fullPath, prefix, uploaded);
    } else if (entry.isFile()) {
      const rel = relative(rootDir, fullPath);
      const dest = `${prefix}/${rel.split("\\").join("/")}`;
      const content = await readFile(fullPath);
      await client.uploadFile(dest, content);
      uploaded.push(dest);
    }
  }
}
