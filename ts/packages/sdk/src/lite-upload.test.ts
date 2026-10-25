// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
	hashLiteSnapshot,
	uploadLiteSnapshot,
	validateLiteStorageURL,
} from "./lite-upload";

describe("Lite snapshot transfer", () => {
	it("hashes in bounded slices", async () => {
		const data = new Blob([new Uint8Array(3 * 1024 * 1024)]);
		const original = data.slice.bind(data);
		data.arrayBuffer = () => {
			throw new Error("whole-file buffering");
		};
		data.slice = (start, end) => {
			expect(end! - start!).toBeLessThanOrEqual(1024 * 1024);
			return original(start, end);
		};
		expect(await hashLiteSnapshot(data)).toHaveLength(64);
		expect(await hashLiteSnapshot(new Blob(["abc"]))).toBe(
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		);
	});
	it("resumes authoritative offset after a lost response without credentials", async () => {
		let stored = 0;
		let writes = 0;
		const size = 10 * 1024 * 1024;
		const transport: typeof fetch = async (_url, options) => {
			expect(options?.credentials).toBe("omit");
			expect(options?.redirect).toBe("error");
			const headers = new Headers(options?.headers);
			expect(headers.has("Authorization")).toBe(false);
			const range = headers.get("Content-Range")!;
			if (range.startsWith("bytes */"))
				return stored === size
					? new Response("{}", { status: 200 })
					: new Response(null, {
							status: 308,
							headers: stored ? { Range: `bytes=0-${stored - 1}` } : {},
						});
			const match = /^bytes (\d+)-(\d+)\//.exec(range)!;
			expect(Number(match[1])).toBe(stored);
			expect((options!.body as Blob).size).toBeLessThanOrEqual(8 * 1024 * 1024);
			writes++;
			if (writes === 1) {
				stored = 1024 * 1024;
				throw new Error("secret-upload-url");
			}
			stored = Number(match[2]) + 1;
			return new Response(null, { status: 308 });
		};
		await uploadLiteSnapshot(
			new Blob([new Uint8Array(size)]),
			"https://storage.googleapis.com/upload?secret=hidden",
			undefined,
			undefined,
			transport,
		);
		expect(stored).toBe(size);
		expect(writes).toBe(3);
	});
	it("rejects foreign URLs, malformed offsets, and redacts transport failures", async () => {
		expect(() =>
			validateLiteStorageURL("https://storage.googleapis.com.evil.test/secret"),
		).toThrow("Invalid transfer URL");
		const file = new Blob(["hello"]);
		await expect(
			uploadLiteSnapshot(
				file,
				"https://storage.googleapis.com/secret",
				undefined,
				undefined,
				async () =>
					new Response(null, {
						status: 308,
						headers: { Range: "bytes=0-900" },
					}),
			),
		).rejects.toThrow("invalid offset");
		await expect(
			uploadLiteSnapshot(
				file,
				"https://storage.googleapis.com/secret",
				undefined,
				undefined,
				async () => {
					throw new Error("secret token");
				},
			),
		).rejects.toThrow("Snapshot transfer interrupted");
	});
});
