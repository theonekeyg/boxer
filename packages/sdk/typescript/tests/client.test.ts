import { describe, expect, it } from "vitest";
import { BoxerClient, BoxerTimeoutError } from "../src/index.js";
import type { ResourceLimits } from "../src/index.js";

const BOXER_URL = process.env.BOXER_URL ?? "";
const IMAGE = "python:3.12-slim";

const needsServer = BOXER_URL ? describe : describe.skip;

needsServer("BoxerClient (integration)", () => {
  const client = new BoxerClient({ baseUrl: BOXER_URL });

  it("health returns true", async () => {
    expect(await client.health()).toBe(true);
  });

  it("run inline command", async () => {
    const result = await client.run(IMAGE, ["python3", "-c", "print(1)"]);
    expect(result.exit_code).toBe(0);
    expect(result.stdout.trim()).toBe("1");
    expect(result.exec_id).toBeTruthy();
  });

  it("non-zero exit code is preserved", async () => {
    const result = await client.run(IMAGE, ["python3", "-c", "exit(1)"]);
    expect(result.exit_code).toBe(1);
  });

  it("upload file and run with files param", async () => {
    const script = new TextEncoder().encode("print('from file')\n");
    const remote = `test_input_${Date.now()}.py`;
    await client.uploadFile(remote, script);

    const result = await client.run(IMAGE, ["python3", `/${remote}`], { files: [remote] });
    expect(result.exit_code).toBe(0);
    expect(result.stdout).toContain("from file");
  });

  it("upload, run with persist, and download output", async () => {
    const script = new TextEncoder().encode(
      "import os; os.makedirs('/output', exist_ok=True); open('/output/result.txt', 'w').write('hello output')\n",
    );
    const remote = `write_output_${Date.now()}.py`;
    await client.uploadFile(remote, script);

    const result = await client.run(IMAGE, ["python3", `/${remote}`], {
      files: [remote],
      persist: true,
    });
    expect(result.exit_code).toBe(0);

    const data = await client.downloadFile(`output/${result.exec_id}/result.txt`);
    expect(new TextDecoder().decode(data)).toBe("hello output");
  });

  it("timeout raises BoxerTimeoutError", async () => {
    const limits: ResourceLimits = { wall_clock_secs: 1 };
    await expect(
      client.run(IMAGE, ["python3", "-c", "while True: pass"], { limits }),
    ).rejects.toBeInstanceOf(BoxerTimeoutError);
  });
});
