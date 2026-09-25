import { useMemo } from "react";
import { sideBySide } from "./lineDiff";

export function DiffView({ left, right, leftTitle, rightTitle }: { left: string; right: string; leftTitle: string; rightTitle: string }) {
  const rows = useMemo(() => sideBySide(left, right), [left, right]);
  const identical = rows.every((r) => r.kind === "same");
  return (
    <div className="of-vh-diff">
      {identical ? <p className="of-ed-hint">These versions have identical content.</p> : null}
      <table aria-label={`Differences between ${leftTitle} and ${rightTitle}`}>
        <thead>
          <tr>
            <th colSpan={2}>{leftTitle}</th>
            <th colSpan={2}>{rightTitle}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className={`of-vh-row of-vh-row--${row.kind}`}>
              <td className="of-vh-ln">{row.left?.n ?? ""}</td>
              <td className="of-vh-code of-vh-code--left">{row.left?.text ?? ""}</td>
              <td className="of-vh-ln">{row.right?.n ?? ""}</td>
              <td className="of-vh-code of-vh-code--right">{row.right?.text ?? ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
