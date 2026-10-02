import { useMemo, useState } from "react";
import { Card, Segmented, Select, Space } from "antd";
import { formatBytes, formatRate } from "@/lib/usage";
import { LiveChart } from "@/components/LiveChart";

export type NetRow = { name: string; rxBytes: number; txBytes: number; rxRate: number; txRate: number };
export type DiskIORow = {
  name: string;
  readBytes: number;
  writeBytes: number;
  readRate: number;
  writeRate: number;
  tps: number;
  ioWaitMs: number;
};
export type IOPoint = {
  t: number;
  net: Record<string, { up: number; down: number }>;
  disk: Record<string, { read: number; write: number; tps: number; wait: number }>;
};

export function snapshotIO(network: NetRow[], diskIO: DiskIORow[]): IOPoint {
  const net: IOPoint["net"] = {};
  for (const n of network || []) net[n.name] = { up: n.txRate || 0, down: n.rxRate || 0 };
  const disk: IOPoint["disk"] = {};
  for (const d of diskIO || []) disk[d.name] = { read: d.readRate || 0, write: d.writeRate || 0, tps: d.tps || 0, wait: d.ioWaitMs || 0 };
  return { t: Date.now(), net, disk };
}

function sumNet(rows: NetRow[], name: string): NetRow {
  const list = name === "all" ? rows : rows.filter((r) => r.name === name);
  return list.reduce(
    (a, r) => ({
      name,
      rxBytes: a.rxBytes + (r.rxBytes || 0),
      txBytes: a.txBytes + (r.txBytes || 0),
      rxRate: a.rxRate + (r.rxRate || 0),
      txRate: a.txRate + (r.txRate || 0),
    }),
    { name, rxBytes: 0, txBytes: 0, rxRate: 0, txRate: 0 },
  );
}

function sumDisk(rows: DiskIORow[], name: string): DiskIORow {
  const list = name === "all" ? rows : rows.filter((r) => r.name === name);
  return list.reduce(
    (a, r) => ({
      name,
      readBytes: a.readBytes + (r.readBytes || 0),
      writeBytes: a.writeBytes + (r.writeBytes || 0),
      readRate: a.readRate + (r.readRate || 0),
      writeRate: a.writeRate + (r.writeRate || 0),
      tps: a.tps + (r.tps || 0),
      ioWaitMs: a.ioWaitMs + (r.ioWaitMs || 0),
    }),
    { name, readBytes: 0, writeBytes: 0, readRate: 0, writeRate: 0, tps: 0, ioWaitMs: 0 },
  );
}

function pickNet(p: IOPoint, name: string) {
  const rows = name === "all" ? Object.values(p.net) : p.net[name] ? [p.net[name]] : [];
  return rows.reduce((a, x) => ({ up: a.up + x.up, down: a.down + x.down }), { up: 0, down: 0 });
}

function pickDisk(p: IOPoint, name: string) {
  const rows = name === "all" ? Object.values(p.disk) : p.disk[name] ? [p.disk[name]] : [];
  return rows.reduce((a, x) => ({ read: a.read + x.read, write: a.write + x.write }), { read: 0, write: 0 });
}

function Stat({ color, label, value }: { color: string; label: string; value: string }) {
  return (
    <div className="rt-stat">
      <span className="rt-dot" style={{ background: color }} />
      <div>
        <div className="rt-stat-label">{label}</div>
        <div className="rt-stat-value">{value}</div>
      </div>
    </div>
  );
}

export function RealtimeIO({
  network,
  diskIO,
  hist,
}: {
  network: NetRow[];
  diskIO: DiskIORow[];
  hist: IOPoint[];
}) {
  const [tab, setTab] = useState<"traffic" | "disk">("traffic");
  const [iface, setIface] = useState("all");
  const [disk, setDisk] = useState("all");
  const net = useMemo(() => sumNet(network || [], iface), [network, iface]);
  const dio = useMemo(() => sumDisk(diskIO || [], disk), [diskIO, disk]);
  const times = hist.map((p) => p.t);
  const netOpts = [{ value: "all", label: "Net: All" }, ...(network || []).map((n) => ({ value: n.name, label: `Net: ${n.name}` }))];
  const diskOpts = [{ value: "all", label: "Disk: All" }, ...(diskIO || []).map((d) => ({ value: d.name, label: `Disk: ${d.name}` }))];

  return (
    <Card
      className="rt-card"
      title={
        <Segmented
          size="small"
          value={tab}
          onChange={(v) => setTab(v as "traffic" | "disk")}
          options={[
            { label: "Traffic", value: "traffic" },
            { label: "Disk IO", value: "disk" },
          ]}
        />
      }
      extra={
        tab === "traffic" ? (
          <Select size="small" style={{ minWidth: 140 }} value={iface} options={netOpts} onChange={setIface} />
        ) : (
          <Select size="small" style={{ minWidth: 140 }} value={disk} options={diskOpts} onChange={setDisk} />
        )
      }
    >
      {tab === "traffic" ? (
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <div className="rt-stats">
            <Stat color="#52c41a" label="Upstream" value={`${formatRate(net.txRate)}/s`} />
            <Stat color="#fa8c16" label="Downstream" value={`${formatRate(net.rxRate)}/s`} />
            <Stat color="#94a3b8" label="Total sent" value={formatBytes(net.txBytes)} />
            <Stat color="#94a3b8" label="Total received" value={formatBytes(net.rxBytes)} />
          </div>
          <LiveChart
            times={times}
            series={[
              { color: "#52c41a", fill: "rgba(82,196,26,0.14)", values: hist.map((p) => pickNet(p, iface).up) },
              { color: "#fa8c16", fill: "rgba(250,140,22,0.12)", values: hist.map((p) => pickNet(p, iface).down) },
            ]}
          />
        </Space>
      ) : (
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <div className="rt-stats">
            <Stat color="#eb2f96" label="Read" value={`${formatRate(dio.readRate)}/s`} />
            <Stat color="#1677ff" label="Write" value={`${formatRate(dio.writeRate)}/s`} />
            <Stat color="#94a3b8" label="TPS" value={Math.round(dio.tps).toLocaleString()} />
            <Stat color="#52c41a" label="IO Wait" value={`${Math.round(dio.ioWaitMs)} ms`} />
          </div>
          <LiveChart
            times={times}
            series={[
              { color: "#eb2f96", fill: "rgba(235,47,150,0.12)", values: hist.map((p) => pickDisk(p, disk).read) },
              { color: "#1677ff", fill: "rgba(22,119,255,0.12)", values: hist.map((p) => pickDisk(p, disk).write) },
            ]}
          />
        </Space>
      )}
    </Card>
  );
}
