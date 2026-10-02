function smoothPath(points: { x: number; y: number }[], closeY: number | null) {
  if (!points.length) return "";
  let d = `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i === 0 ? 0 : i - 1];
    const p1 = points[i];
    const p2 = points[i + 1];
    const p3 = points[i + 2] || p2;
    const c1x = p1.x + (p2.x - p0.x) / 6;
    const c1y = p1.y + (p2.y - p0.y) / 6;
    const c2x = p2.x - (p3.x - p1.x) / 6;
    const c2y = p2.y - (p3.y - p1.y) / 6;
    d += ` C ${c1x.toFixed(1)} ${c1y.toFixed(1)}, ${c2x.toFixed(1)} ${c2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
  }
  if (closeY != null) {
    d += ` L ${points[points.length - 1].x.toFixed(1)} ${closeY} L ${points[0].x.toFixed(1)} ${closeY} Z`;
  }
  return d;
}

function clock(ts: number) {
  const d = new Date(ts);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

export function LiveChart({
  times,
  series,
  height = 196,
}: {
  times: number[];
  series: { color: string; fill: string; values: number[] }[];
  height?: number;
}) {
  const w = 720;
  const pad = { l: 8, r: 8, t: 14, b: 26 };
  const innerW = w - pad.l - pad.r;
  const innerH = height - pad.t - pad.b;
  const n = Math.max(2, times.length);
  const max = Math.max(0.001, ...series.flatMap((s) => s.values));
  const pts = (values: number[]) =>
    values.map((v, i) => ({
      x: pad.l + (i / (n - 1)) * innerW,
      y: pad.t + innerH - (Math.max(0, v) / max) * innerH,
    }));
  const labels = times.length
    ? [times[0], times[Math.floor((times.length - 1) / 2)], times[times.length - 1]]
    : [];

  return (
    <svg className="live-chart" viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" role="img">
      <line className="live-chart-grid" x1={pad.l} x2={w - pad.r} y1={pad.t + innerH * 0.25} y2={pad.t + innerH * 0.25} />
      <line className="live-chart-grid" x1={pad.l} x2={w - pad.r} y1={pad.t + innerH * 0.5} y2={pad.t + innerH * 0.5} />
      <line className="live-chart-grid" x1={pad.l} x2={w - pad.r} y1={pad.t + innerH * 0.75} y2={pad.t + innerH * 0.75} />
      {series.map((s) => {
        const p = pts(s.values.length ? s.values : [0, 0]);
        return (
          <g key={s.color}>
            <path d={smoothPath(p, pad.t + innerH)} fill={s.fill} />
            <path d={smoothPath(p, null)} fill="none" stroke={s.color} strokeWidth="2" />
            {p.length ? <circle cx={p[p.length - 1].x} cy={p[p.length - 1].y} r="3.2" fill={s.color} /> : null}
          </g>
        );
      })}
      {labels.map((t, i) => (
        <text
          key={t + i}
          className="live-chart-label"
          x={i === 0 ? pad.l : i === 2 ? w - pad.r : w / 2}
          y={height - 6}
          textAnchor={i === 0 ? "start" : i === 2 ? "end" : "middle"}
        >
          {clock(t)}
        </text>
      ))}
    </svg>
  );
}
