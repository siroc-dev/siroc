export function shouldReloadForVersion(previous: string, next: string, sawDown: boolean) {
  if (!next) return false;
  if (previous && next !== previous) return true;
  return sawDown;
}
