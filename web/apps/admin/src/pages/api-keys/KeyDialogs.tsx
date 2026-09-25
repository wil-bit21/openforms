import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ApiKey } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { CopyButton } from "../../components/CopyButton";
import { Dialog } from "../../components/Dialog";
import { ErrorMessage } from "../../components/ErrorMessage";
import { parseRoles } from "../../lib/roles";

export function CreateKeyDialog({ onCreated, onClose }: { onCreated: (plaintext: string) => void; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [roles, setRoles] = useState("");
  const mutation = useMutation({
    mutationFn: () => client.createApiKey({ name: name.trim(), roles: parseRoles(roles) }),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys });
      onCreated(res.key);
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    mutation.mutate();
  }

  return (
    <Dialog title="Create API key" onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="key-name">Name</label>
          <input id="key-name" required value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="key-roles">Roles</label>
          <input id="key-roles" value={roles} onChange={(e) => setRoles(e.target.value)} aria-describedby="key-roles-help" />
          <p id="key-roles-help" className="field-help">
            Comma-separated. Use <code>admin</code> for <code>openforms push</code>; workflow roles for integrations that move submissions.
          </p>
        </div>
        {mutation.isError && <ErrorMessage error={mutation.error} />}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={mutation.isPending}>Create key</button>
        </div>
      </form>
    </Dialog>
  );
}

export function ShowKeyDialog({ plaintext, onClose }: { plaintext: string; onClose: () => void }) {
  return (
    <Dialog title="Copy your API key" onClose={onClose}>
      <p className="notice">Store it somewhere safe now: you won't be able to see it again.</p>
      <div className="field">
        <label htmlFor="key-plaintext">API key</label>
        <input id="key-plaintext" className="key-display" readOnly value={plaintext} onFocus={(e) => e.currentTarget.select()} />
      </div>
      <div className="dialog-footer">
        <CopyButton text={plaintext} />
        <button type="button" className="btn btn-primary" onClick={onClose}>Done</button>
      </div>
    </Dialog>
  );
}

export function RevokeKeyDialog({ apiKey, onClose }: { apiKey: ApiKey; onClose: () => void }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => client.revokeApiKey(apiKey.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys });
      onClose();
    },
  });
  return (
    <Dialog title={`Revoke ${apiKey.name}?`} onClose={onClose}>
      <p>Requests using this key will be rejected immediately. This cannot be undone.</p>
      {mutation.isError && <ErrorMessage error={mutation.error} />}
      <div className="dialog-footer">
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn-danger" disabled={mutation.isPending} onClick={() => mutation.mutate()}>Revoke key</button>
      </div>
    </Dialog>
  );
}
