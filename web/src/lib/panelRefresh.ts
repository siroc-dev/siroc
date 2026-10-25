export function shouldReloadForVersion(previous: string, next: string, sawDown: boolean, agentOk: boolean) {
  if (!agentOk || !next) return false;
  if (previous && next !== previous) return true;
  return sawDown;
}
