import { relativeTime } from "../lib/format";

export function RelativeTime({ iso }: { iso: string }) {
  return (
    <time dateTime={iso} title={new Date(iso).toLocaleString()}>
      {relativeTime(iso)}
    </time>
  );
}
