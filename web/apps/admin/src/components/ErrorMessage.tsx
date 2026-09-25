import { errorMessage } from "../lib/errors";

export function ErrorMessage({ error }: { error: unknown }) {
  return <p role="alert" className="error">{errorMessage(error)}</p>;
}
