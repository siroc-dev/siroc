import { Checkbox, Typography } from "antd";

export type CheckOption = { key: string; group: string; exclusive?: boolean; label: string; detail: string };

const groupTitle: Record<string, string> = {
  access: "Access",
  commit: "Write commit",
  identity: "Identity",
  subtree: "Subtree",
  ports: "Client port",
  extra: "Extra",
  retry: "When the server is down",
  startup: "First attempt",
  boot: "Boot",
  safety: "Local safety",
  atime: "Access time",
  dirtime: "Directories",
  ssd: "Solid-state disk",
  errors: "Errors",
  owner: "Who can mount",
};

export function toggleFlag(options: CheckOption[], selected: string[], opt: CheckOption, on: boolean) {
  let next = selected.filter((key) => key !== opt.key);
  if (on) {
    if (opt.exclusive && opt.group) {
      const same = new Set(options.filter((item) => item.group === opt.group).map((item) => item.key));
      next = next.filter((key) => !same.has(key));
    }
    next.push(opt.key);
  }
  return next;
}

export function OptionList({ options, value, onChange }: { options: CheckOption[]; value: string[]; onChange: (next: string[]) => void }) {
  const groups: { key: string; items: CheckOption[] }[] = [];
  for (const opt of options) {
    const key = opt.group || "extra";
    let bucket = groups.find((item) => item.key === key);
    if (!bucket) {
      bucket = { key, items: [] };
      groups.push(bucket);
    }
    bucket.items.push(opt);
  }
  return (
    <div className="nfs-options">
      {groups.map((group) => (
        <div key={group.key}>
          <Typography.Text strong>{groupTitle[group.key] || group.key}</Typography.Text>
          {group.items.map((opt) => (
            <div key={opt.key} className="nfs-option">
              <Checkbox checked={value.includes(opt.key)} onChange={(e) => onChange(toggleFlag(options, value, opt, e.target.checked))}>
                {opt.label}
              </Checkbox>
              <span className="nfs-option-detail">{opt.detail}</span>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
