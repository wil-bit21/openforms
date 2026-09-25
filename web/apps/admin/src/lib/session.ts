import { useQuery } from "@tanstack/react-query";
import type { Principal } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";

export function useSession() {
  return useQuery({ queryKey: qk.me, queryFn: () => client.me(), retry: false, staleTime: 5 * 60_000 });
}

export function isAdmin(p?: Principal | null): boolean {
  return !!p && p.roles.includes("admin");
}
