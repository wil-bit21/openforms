import type { ReactNode } from "react";

export function Notice({ title, tone = "info", children }: { title: string; tone?: "info" | "warn"; children: ReactNode }) {
  return (
    <div className={`notice notice-${tone}`} role="status">
      <strong>{title}</strong>
      <div>{children}</div>
    </div>
  );
}
