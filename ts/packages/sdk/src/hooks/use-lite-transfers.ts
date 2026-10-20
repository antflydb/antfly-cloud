import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";
import {
	hashLiteSnapshot,
	type LiteUploadProgress,
	uploadLiteSnapshot,
	validateLiteStorageURL,
} from "../lite-upload";
import type { components } from "../types";

export type LiteTransferOperation =
	components["schemas"]["LiteTransferOperation"];
export type ConfirmLiteImportRequest =
	components["schemas"]["ConfirmLiteImportRequest"];
export type { LiteUploadProgress } from "../lite-upload";

function checked<T>(result: {
	data?: T;
	error?: unknown;
	response: Response;
}): T {
	if (!result.response.ok || !result.data) {
		// Never surface raw transport errors or signed URLs in UI notifications.
		const detail =
			typeof result.error === "object" &&
			result.error !== null &&
			"detail" in result.error
				? String(result.error.detail)
				: "Lite request failed. Check access and transfer status before retrying.";
		throw new Error(detail);
	}
	return result.data;
}
export function useLiteCapabilities(orgId: string | undefined) {
	return useQuery({
		queryKey: ["lite-capabilities", orgId],
		enabled: !!orgId,
		retry: false,
		staleTime: 60000,
		queryFn: async () =>
			checked(
				await client.GET("/organizations/{org_id}/cloud/capabilities", {
					params: { path: { org_id: orgId! } },
				}),
			),
	});
}
const terminal = new Set([
	"awaiting_confirmation",
	"ready",
	"download_ready",
	"failed",
	"cancelled",
	"expired",
]);
export function useLiteOperation(
	orgId: string | undefined,
	operationId: string | undefined,
) {
	return useQuery({
		queryKey: ["lite-operation", orgId, operationId],
		enabled: !!orgId && !!operationId,
		retry: false,
		refetchInterval: (query) =>
			query.state.data && terminal.has(query.state.data.phase) ? false : 2000,
		queryFn: async () =>
			checked(
				await client.GET(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}",
					{ params: { path: { org_id: orgId!, operation_id: operationId! } } },
				),
			),
	});
}
export function useUploadLiteSnapshot(orgId: string | undefined) {
	const cache = useQueryClient();
	return useMutation({
		mutationFn: async (input: {
			file: File;
			key: string;
			onProgress?: (progress: LiteUploadProgress) => void;
			onAdmitted?: (operation: LiteTransferOperation) => void;
			signal?: AbortSignal;
		}) => {
			if (!orgId || input.file.size === 0)
				throw new Error("Select a nonempty Lite snapshot.");
			const digest = await hashLiteSnapshot(
				input.file,
				input.onProgress,
				input.signal,
			);
			const operation = checked(
				await client.POST("/organizations/{org_id}/cloud/lite/imports", {
					params: { path: { org_id: orgId } },
					body: {
						idempotency_key: input.key,
						size_bytes: input.file.size,
						sha256: digest,
					},
					signal: input.signal,
				}),
			);
			input.onAdmitted?.(operation);
			if (operation.phase !== "uploading") return operation;
			const path = { org_id: orgId, operation_id: operation.id };
			const session = checked(
				await client.POST(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}/upload-session",
					{ params: { path }, signal: input.signal },
				),
			);
			await uploadLiteSnapshot(
				input.file,
				session.upload_url,
				input.onProgress,
				input.signal,
			);
			return checked(
				await client.POST(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}/seal",
					{ params: { path }, signal: input.signal },
				),
			);
		},
		onSuccess: (operation) =>
			cache.invalidateQueries({
				queryKey: ["lite-operation", orgId, operation.id],
			}),
	});
}
export function useConfirmLiteImport(
	orgId: string | undefined,
	operationId: string | undefined,
) {
	const cache = useQueryClient();
	return useMutation({
		mutationFn: async (body: ConfirmLiteImportRequest) =>
			checked(
				await client.POST(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}/confirm",
					{
						params: { path: { org_id: orgId!, operation_id: operationId! } },
						body,
					},
				),
			),
		onSuccess: () => {
			void cache.invalidateQueries({
				queryKey: ["lite-operation", orgId, operationId],
			});
			void cache.invalidateQueries({
				queryKey: ["organizations", orgId, "cloud-instances"],
			});
		},
	});
}
export function useCancelLiteTransfer(orgId: string | undefined) {
	const cache = useQueryClient();
	return useMutation({
		mutationFn: async (id: string) =>
			checked(
				await client.POST(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}/cancel",
					{ params: { path: { org_id: orgId!, operation_id: id } } },
				),
			),
		onSuccess: (operation) =>
			cache.invalidateQueries({
				queryKey: ["lite-operation", orgId, operation.id],
			}),
	});
}
export function useExportLiteSnapshot(
	orgId: string | undefined,
	instanceId: string,
) {
	return useMutation({
		mutationFn: async (key: string) =>
			checked(
				await client.POST(
					"/organizations/{org_id}/cloud/instances/{instance_id}/lite/exports",
					{
						params: { path: { org_id: orgId!, instance_id: instanceId } },
						body: { idempotency_key: key },
					},
				),
			),
	});
}
export function useDownloadLiteSnapshot(orgId: string | undefined) {
	return useMutation({
		mutationFn: async (id: string) => {
			const result = checked(
				await client.POST(
					"/organizations/{org_id}/cloud/lite/operations/{operation_id}/download",
					{ params: { path: { org_id: orgId!, operation_id: id } } },
				),
			);
			validateLiteStorageURL(result.download_url);
			return result;
		},
		gcTime: 0,
	});
}

export function useResumeLite(orgId: string | undefined, instanceId: string) {
  const cache = useQueryClient();
  return useMutation({
    mutationFn: async () => checked(await client.POST(
      "/organizations/{org_id}/cloud/instances/{instance_id}/resume",
      {params: {path: {org_id: orgId!, instance_id: instanceId}}},
    )),
    onSuccess: (instance) => {
      cache.setQueryData(["organizations", orgId, "cloud-instances", instanceId], instance);
      void cache.invalidateQueries({queryKey: ["organizations", orgId, "cloud-instances"]});
    },
  });
}
