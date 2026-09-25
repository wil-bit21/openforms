import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { client } from "../api";
import { errorMessage } from "../lib/errors";

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const request = useMutation({ mutationFn: () => client.requestPasswordReset(email.trim()) });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    request.mutate();
  }

  return (
    <main className="login">
      <form className="card login-card" onSubmit={onSubmit}>
        <h1>Reset your password</h1>
        {request.isSuccess ? (
          <p role="status">
            If an account exists for <strong>{email.trim()}</strong>, we sent it a link to choose a new password. The link
            expires in 1 hour.
          </p>
        ) : (
          <>
            <p className="muted">Enter the email you sign in with and we will send you a reset link.</p>
            {request.isError && (
              <p role="alert" className="error">
                {errorMessage(request.error)}
              </p>
            )}
            <div className="field">
              <label htmlFor="forgot-email">Email</label>
              <input id="forgot-email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <button type="submit" className="btn btn-primary" disabled={request.isPending}>
              {request.isPending ? "Sending…" : "Send reset link"}
            </button>
          </>
        )}
        <Link to="/login">Back to sign in</Link>
      </form>
    </main>
  );
}
