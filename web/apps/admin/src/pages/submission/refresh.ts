import { useQueryClient } from "@tanstack/react-query";
import { qk } from "../../queryKeys";

export function useRefreshSubmission(id: string) {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: qk.submission(id) }),
      queryClient.invalidateQueries({ queryKey: ["submissions"] }),
    ]);
}
