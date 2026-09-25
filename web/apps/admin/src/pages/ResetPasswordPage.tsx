import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { errorCode, errorMessage, problemsByPath } from "../lib/errors";

export function ResetPasswordPage() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [mismatch, setMismatch] = useState(false);
  const reset = useMutation({ mutationFn: () => client.confirmPasswordReset(token, password) });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    setMismatch(password !== confirm);
    if (password === confirm) reset.mutate();
  }

  const invalidLink = !token || errorCode(reset.error) === "invalid_token";

  return (
    <main className="login">
      <div className="card login-card">
        <h1>Choose a new password</h1>
        {reset.isSuccess ? (
          <>
            <p role="status">Your password was changed and you were signed out everywhere.</p>
            <Link className="btn btn-primary" to="/login">
              Sign in
            </Link>
          </>
        ) : invalidLink ? (
          <>
            <p role="alert" className="error">
              This reset link is invalid or has expired.
            </p>
            <Link to="/forgot-password">Request a new link</Link>
          </>
        ) : (
          <form className="login-card" onSubmit={onSubmit}>
            {(mismatch || reset.isError) && (
              <p role="alert" className="error">
                {mismatch ? "The passwords do not match." : problemsByPath(reset.error).password ? `Password ${problemsByPath(reset.error).password}.` : errorMessage(reset.error)}
              </p>
            )}
            <div className="field">
              <label htmlFor="reset-password">New password</label>
              <input id="reset-password" type="password" autoComplete="new-password" minLength={8} required value={password} onChange={(e) => setPassword(e.target.value)} />
            </div>
            <div className="field">
              <label htmlFor="reset-confirm">Confirm new password</label>
              <input id="reset-confirm" type="password" autoComplete="new-password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </div>
            <button type="submit" className="btn btn-primary" disabled={reset.isPending}>
              {reset.isPending ? "Saving…" : "Set new password"}
            </button>
          </form>
        )}
      </div>
    </main>
  );
}
