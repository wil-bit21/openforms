import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { useRefreshSubmission } from "./refresh";

export function CommentBox({ submissionId }: { submissionId: string }) {
  const [body, setBody] = useState("");
  const refresh = useRefreshSubmission(submissionId);
  const mutation = useMutation({
    mutationFn: () => client.comment(submissionId, body.trim()),
    onSuccess: async () => {
      setBody("");
      await refresh();
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    if (body.trim()) mutation.mutate();
  }

  return (
    <form className="comment-box" onSubmit={submit}>
      <label htmlFor="comment-body" className="sr-only">Comment</label>
      <textarea id="comment-body" rows={3} placeholder="Add a note for your team" value={body} onChange={(e) => setBody(e.target.value)} />
      {mutation.isError && <ErrorMessage error={mutation.error} />}
      <button type="submit" className="btn" disabled={!body.trim() || mutation.isPending}>Add comment</button>
    </form>
  );
}
