import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { BoxerClient } from "../src/index.js";
import { uploadPath } from "../src/node.js";

const BOXER_URL = process.env.BOXER_URL ?? "";
const IMAGE = "python:3.12-slim";

const needsServer = BOXER_URL ? describe : describe.skip;

needsServer("uploadPath (integration)", () => {
  const client = new BoxerClient({ baseUrl: BOXER_URL });

  it("uploads a single file", async () => {
    const dir = await mkdtemp(join(tmpdir(), "boxer-test-"));
    const file = join(dir, "hello.txt");
    await writeFile(file, "hello from uploadPath");

    const paths = await uploadPath(client, file);
    expect(paths).toEqual(["hello.txt"]);

    const result = await client.run(
      IMAGE,
      ["python3", "-c", "import os; print(os.path.exists('/hello.txt'))"],
      { files: paths },
    );
    expect(result.exit_code).toBe(0);
    expect(result.stdout.trim()).toBe("True");
  });

  it("uploads a directory recursively", async () => {
    const dir = await mkdtemp(join(tmpdir(), "boxer-test-"));
    await writeFile(join(dir, "a.txt"), "file a");
    await mkdir(join(dir, "sub"));
    await writeFile(join(dir, "sub", "b.txt"), "file b");

    const paths = await uploadPath(client, dir, "mydir");
    expect(paths.sort()).toEqual(["mydir/a.txt", "mydir/sub/b.txt"]);

    const result = await client.run(
      IMAGE,
      [
        "python3",
        "-c",
        "import os; print(os.path.exists('/mydir/a.txt') and os.path.exists('/mydir/sub/b.txt'))",
      ],
      { files: paths },
    );
    expect(result.exit_code).toBe(0);
    expect(result.stdout.trim()).toBe("True");
  });
});
