import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { qk } from "../queryKeys";

export function isUnauthenticated(err: unknown): boolean {
  return err instanceof OpenFormsError && err.status === 401;
}

/**
 * Any 401 from a non-session query or mutation invalidates the session query;
 * RequireSession then refetches /auth/me, gets 401 and redirects to /login.
 */
export function createQueryClient(opts: { test?: boolean } = {}): QueryClient {
  const onAuthError = (err: unknown) => {
    if (isUnauthenticated(err)) void queryClient.invalidateQueries({ queryKey: qk.me });
  };
  const queryClient: QueryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (err, query) => {
        if (query.queryKey[0] !== qk.me[0]) onAuthError(err);
      },
    }),
    mutationCache: new MutationCache({ onError: (err) => onAuthError(err) }),
    defaultOptions: {
      queries: {
        retry: opts.test
          ? false
          : (count, err) => !(err instanceof OpenFormsError && err.status < 500) && count < 2,
        refetchOnWindowFocus: !opts.test,
      },
      mutations: { retry: false },
    },
  });
  return queryClient;
}
