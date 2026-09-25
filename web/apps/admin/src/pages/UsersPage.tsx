import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { User } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";
import { useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { RoleTags } from "../components/RoleTags";
import { DeleteUserDialog, UserDialog } from "./users/UserDialogs";

type Open = { kind: "create" } | { kind: "edit"; user: User } | { kind: "delete"; user: User } | null;

export function UsersPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.users, queryFn: () => client.listUsers() });
  const [open, setOpen] = useState<Open>(null);
  const close = () => setOpen(null);

  return (
    <div className="page">
      <header className="page-header">
        <h1>Users</h1>
        <button type="button" className="btn btn-primary" onClick={() => setOpen({ kind: "create" })}>Add user</button>
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Roles</th>
              <th>Created</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((u) => (
              <tr key={u.id}>
                <td>{u.name}</td>
                <td>{u.email}</td>
                <td><RoleTags roles={u.roles} /></td>
                <td><RelativeTime iso={u.createdAt} /></td>
                <td>
                  <div className="button-row">
                    <button type="button" className="btn" onClick={() => setOpen({ kind: "edit", user: u })}>Edit</button>
                    <button type="button" className="btn" disabled={me?.id === u.id}
                      title={me?.id === u.id ? "You can't delete your own account." : undefined}
                      onClick={() => setOpen({ kind: "delete", user: u })}>
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {open?.kind === "create" && <UserDialog onClose={close} />}
      {open?.kind === "edit" && <UserDialog user={open.user} onClose={close} />}
      {open?.kind === "delete" && <DeleteUserDialog user={open.user} onClose={close} />}
    </div>
  );
}
