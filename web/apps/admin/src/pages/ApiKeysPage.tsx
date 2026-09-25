import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ApiKey } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { RoleTags } from "../components/RoleTags";
import { CreateKeyDialog, RevokeKeyDialog, ShowKeyDialog } from "./api-keys/KeyDialogs";

type Open = { kind: "create" } | { kind: "show"; plaintext: string } | { kind: "revoke"; apiKey: ApiKey } | null;

export function ApiKeysPage() {
  const query = useQuery({ queryKey: qk.apiKeys, queryFn: () => client.listApiKeys() });
  const [open, setOpen] = useState<Open>(null);
  const close = () => setOpen(null);

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1>API keys</h1>
          <p className="muted">Keys authenticate the CLI and your integrations with <code>Authorization: Bearer ofk_…</code>.</p>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => setOpen({ kind: "create" })}>Create API key</button>
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No API keys yet.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Key</th>
              <th>Roles</th>
              <th>Created</th>
              <th>Last used</th>
              <th>Status</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((k) => (
              <tr key={k.id}>
                <td>{k.name}</td>
                <td className="mono">{k.prefix}…</td>
                <td><RoleTags roles={k.roles} /></td>
                <td><RelativeTime iso={k.createdAt} /></td>
                <td>{k.lastUsedAt ? <RelativeTime iso={k.lastUsedAt} /> : <span className="muted">Never</span>}</td>
                <td>{k.revokedAt ? <span className="badge badge-red">Revoked</span> : <span className="badge badge-green">Active</span>}</td>
                <td>
                  {!k.revokedAt && (
                    <button type="button" className="btn" onClick={() => setOpen({ kind: "revoke", apiKey: k })}>Revoke</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {open?.kind === "create" && <CreateKeyDialog onClose={close} onCreated={(plaintext) => setOpen({ kind: "show", plaintext })} />}
      {open?.kind === "show" && <ShowKeyDialog plaintext={open.plaintext} onClose={close} />}
      {open?.kind === "revoke" && <RevokeKeyDialog apiKey={open.apiKey} onClose={close} />}
    </div>
  );
}
