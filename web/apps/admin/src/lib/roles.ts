export function parseRoles(input: string): string[] {
  const out: string[] = [];
  for (const part of input.split(",")) {
    const role = part.trim();
    if (role && !out.includes(role)) out.push(role);
  }
  return out;
}

export function formatRoles(roles: string[]): string {
  return roles.join(", ");
}
