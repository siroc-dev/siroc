let zone = "";

export function setPanelTimezone(next?: string) {
  zone = (next || "").trim();
}

export function panelTimezone() {
  return zone;
}

function panelDate(raw?: string) {
  if (!raw || raw.startsWith("0001")) return null;
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return null;
  return d;
}

function panelParts(d: Date) {
  const opts: Intl.DateTimeFormatOptions = {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  };
  if (zone) opts.timeZone = zone;
  return new Intl.DateTimeFormat("en-GB", opts).formatToParts(d);
}

function part(parts: Intl.DateTimeFormatPart[], type: Intl.DateTimeFormatPartTypes) {
  return parts.find((p) => p.type === type)?.value || "";
}

export function formatPanelTime(raw?: string) {
  const d = panelDate(raw);
  if (!d) return "";
  try {
    const p = panelParts(d);
    const hour = part(p, "hour").padStart(2, "0");
    return `${part(p, "year")}-${part(p, "month")}-${part(p, "day")} ${hour}:${part(p, "minute")}:${part(p, "second")}`;
  } catch {
    return raw || "";
  }
}
