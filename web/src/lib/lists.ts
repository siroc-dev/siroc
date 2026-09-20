export function asList<T>(v: T[] | null | undefined): T[] {
  return Array.isArray(v) ? v : [];
}
