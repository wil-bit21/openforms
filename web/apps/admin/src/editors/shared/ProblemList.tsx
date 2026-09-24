import type { Problem } from "./types";

export function ProblemList({ problems, onSelect }: { problems: Problem[]; onSelect?: (problem: Problem) => void }) {
  if (problems.length === 0) return null;
  return (
    <section className="of-ed-problems" aria-label="Problems">
      <h2>{problems.length === 1 ? "1 problem" : `${problems.length} problems`}</h2>
      <ul>
        {problems.map((p, i) => {
          const content = (
            <>
              <code>{p.path || "(definition)"}</code> {p.message}
            </>
          );
          return (
            <li key={`${p.path}-${i}`}>
              {onSelect ? (
                <button type="button" className="of-ed-link" onClick={() => onSelect(p)}>
                  {content}
                </button>
              ) : (
                content
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
