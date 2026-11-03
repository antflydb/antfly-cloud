import { sha256 } from "@noble/hashes/sha2.js";
import { bytesToHex } from "@noble/hashes/utils.js";

export type LiteUploadProgress = {
	phase: "hashing" | "uploading";
	bytes: number;
	total: number;
};
const chunkBytes = 8 * 1024 * 1024;

function uploadStatus(response: Response): number {
	const override = response.headers.get("X-HTTP-Status-Code-Override");
	if (override === null) return response.status;
	if (response.status !== 200 || override !== "308")
		throw new Error("Upload session returned an invalid status override.");
	return 308;
}

/** Hash a prepared snapshot without buffering the database in memory. */
export async function hashLiteSnapshot(
	file: Blob,
	progress?: (value: LiteUploadProgress) => void,
	signal?: AbortSignal,
): Promise<string> {
	const hash = sha256.create();
	for (let offset = 0; offset < file.size; offset += 1024 * 1024) {
		signal?.throwIfAborted();
		hash.update(
			new Uint8Array(
				await file.slice(offset, offset + 1024 * 1024).arrayBuffer(),
			),
		);
		progress?.({
			phase: "hashing",
			bytes: Math.min(offset + 1024 * 1024, file.size),
			total: file.size,
		});
		await new Promise<void>((resolve) => setTimeout(resolve, 0));
	}
	return bytesToHex(hash.digest());
}

/** Scoped URLs are bearer capabilities: no cookies, Cloud headers, redirects or URL-bearing errors. */
export function validateLiteStorageURL(value: string): string {
	let url: URL;
	try {
		url = new URL(value);
	} catch {
		throw new Error("Invalid transfer URL. Request a new transfer session.");
	}
	if (
		url.protocol !== "https:" ||
		url.hostname !== "storage.googleapis.com" ||
		url.port ||
		url.username ||
		url.password ||
		url.hash
	) {
		throw new Error("Invalid transfer URL. Request a new transfer session.");
	}
	return value;
}

export async function uploadLiteSnapshot(
	file: Blob,
	session: string,
	progress?: (value: LiteUploadProgress) => void,
	signal?: AbortSignal,
	transport: typeof fetch = fetch,
): Promise<void> {
	const url = validateLiteStorageURL(session);
	async function request(
		headers: Record<string, string>,
		body?: Blob,
	): Promise<Response> {
		try {
			return await transport(url, {
				method: "PUT",
				// Fetch rejects HTTP 308 with redirect:error, even without Location.
				// GCS exposes its resumable status in this header instead.
				headers: { ...headers, "X-GUploader-No-308": "yes" },
				body,
				signal,
				credentials: "omit",
				redirect: "error",
				referrerPolicy: "no-referrer",
			});
		} catch {
			signal?.throwIfAborted();
			throw new Error(
				"Snapshot transfer interrupted. Resume with the same snapshot and transfer key.",
			);
		}
	}
	async function committed(): Promise<number> {
		const response = await request({ "Content-Range": `bytes */${file.size}` });
		const status = uploadStatus(response);
		if (status === 200 || status === 201) return file.size;
		if (status !== 308)
			throw new Error(
				`Upload session is unavailable (${status}). Resume or cancel this transfer.`,
			);
		const range = response.headers.get("Range");
		if (range === null) return 0;
		const match = /^bytes=0-(\d+)$/.exec(range);
		const offset = match ? Number(match[1]) + 1 : Number.NaN;
		if (!Number.isSafeInteger(offset) || offset < 0 || offset > file.size)
			throw new Error("Upload session returned an invalid offset.");
		return offset;
	}
	let offset = await committed();
	let stalls = 0;
	while (offset < file.size) {
		const end = Math.min(offset + chunkBytes, file.size);
		try {
			const response = await request(
				{
					"Content-Type": "application/octet-stream",
					"Content-Range": `bytes ${offset}-${end - 1}/${file.size}`,
				},
				file.slice(offset, end),
			);
			if (![200, 201, 308].includes(uploadStatus(response)))
				throw new Error("Upload chunk was not accepted.");
		} catch {
			signal?.throwIfAborted(); /* Query authoritative offset after an ambiguous response. */
		}
		const next = await committed();
		if (next < offset || (next > end && next !== file.size))
			throw new Error(
				"Upload offset changed unexpectedly. Use one uploader per transfer.",
			);
		stalls = next === offset ? stalls + 1 : 0;
		if (stalls >= 5)
			throw new Error(
				"Upload made no progress. Resume with the same snapshot and transfer key.",
			);
		offset = next;
		progress?.({ phase: "uploading", bytes: offset, total: file.size });
	}
}
