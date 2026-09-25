import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { User } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { Dialog } from "../../components/Dialog";
import { errorCode, errorMessage, problemsByPath } from "../../lib/errors";
import { formatRoles, parseRoles } from "../../lib/roles";

export function UserDialog({ user, onClose }: { user?: User; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const [password, setPassword] = useState("");
  const [roles, setRoles] = useState(formatRoles(user?.roles ?? []));

  const mutation = useMutation({
    mutationFn: async () => {
      if (!user) {
        return client.createUser({ email: email.trim(), name: name.trim(), password, roles: parseRoles(roles) });
      }
      const input: { name?: string; password?: string; roles?: string[] } = {};
      if (name.trim() !== user.name) input.name = name.trim();
      if (password) input.password = password;
      const nextRoles = parseRoles(roles);
      if (nextRoles.join(",") !== user.roles.join(",")) input.roles = nextRoles;
      return client.updateUser(user.id, input);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.users });
      onClose();
    },
  });

  const problems = problemsByPath(mutation.error);
  const alertText = !mutation.isError
    ? null
    : errorCode(mutation.error) === "email_taken"
      ? "A user with this email already exists."
      : Object.keys(problems).length === 0
        ? errorMessage(mutation.error)
        : null;

  function submit(e: FormEvent) {
    e.preventDefault();
    mutation.mutate();
  }

  return (
    <Dialog title={user ? `Edit ${user.name}` : "Add user"} onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="user-name">Name</label>
          <input id="user-name" required value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        {!user && (
          <div className="field">
            <label htmlFor="user-email">Email</label>
            <input id="user-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
        )}
        <div className="field">
          <label htmlFor="user-password">{user ? "New password" : "Password"}</label>
          <input id="user-password" type="password" autoComplete="new-password" minLength={8} required={!user}
            value={password} onChange={(e) => setPassword(e.target.value)} aria-describedby="user-password-help" />
          <p id="user-password-help" className="field-help">
            {user ? "Leave blank to keep the current password." : "At least 8 characters."}
          </p>
        </div>
        <div className="field">
          <label htmlFor="user-roles">Roles</label>
          <input id="user-roles" value={roles} onChange={(e) => setRoles(e.target.value)} aria-describedby="user-roles-help" />
          <p id="user-roles-help" className="field-help">
            Comma-separated, e.g. <code>reviewer, hiring-manager</code>. <code>admin</code> can do everything.
          </p>
        </div>
        {Object.entries(problems).map(([path, message]) => (
          <p key={path} className="field-error">{message}</p>
        ))}
        {alertText && <p role="alert" className="error">{alertText}</p>}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={mutation.isPending}>{user ? "Save" : "Add user"}</button>
        </div>
      </form>
    </Dialog>
  );
}

export function DeleteUserDialog({ user, onClose }: { user: User; onClose: () => void }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => client.deleteUser(user.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.users });
      onClose();
    },
  });
  return (
    <Dialog title={`Delete ${user.name}?`} onClose={onClose}>
      <p>They lose access immediately. Submissions assigned to them become unassigned; their history stays.</p>
      {mutation.isError && <p role="alert" className="error">{errorMessage(mutation.error)}</p>}
      <div className="dialog-footer">
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn-danger" disabled={mutation.isPending} onClick={() => mutation.mutate()}>Delete user</button>
      </div>
    </Dialog>
  );
}
