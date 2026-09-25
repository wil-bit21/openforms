import { useState, type FormEvent } from "react";
import { DemoCredentials } from "../components/DemoCredentials";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode, errorMessage } from "../lib/errors";

export function safeNext(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return "/submissions";
  return next;
}

export function LoginPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: () => client.login(email.trim(), password),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: qk.me });
      navigate(safeNext(params.get("next")), { replace: true });
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate();
  }

  return (
    <main className="login">
      <form className="card login-card" onSubmit={onSubmit}>
        <h1>Sign in to openforms</h1>
        {login.isError && (
          <p role="alert" className="error">
            {errorCode(login.error) === "invalid_credentials"
              ? "Email or password is incorrect."
              : errorCode(login.error) === "rate_limited"
                ? "Too many sign-in attempts. Wait a few minutes and try again."
                : errorMessage(login.error)}
          </p>
        )}
        <div className="field">
          <label htmlFor="login-email">Email</label>
          <input id="login-email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="login-password">Password</label>
          <input id="login-password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <button type="submit" className="btn btn-primary" disabled={login.isPending}>
          {login.isPending ? "Signing in…" : "Sign in"}
        </button>
        <Link to="/forgot-password">Forgot your password?</Link>
      </form>
      <DemoCredentials
        onPick={(email, password) => {
          setEmail(email);
          setPassword(password);
        }}
      />
    </main>
  );
}
