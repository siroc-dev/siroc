import { Typography } from "antd";
import { AccountWAF } from "@/components/AccountWAF";

export function UserSecurity() {
  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Security
        </Typography.Title>
        <Typography.Paragraph type="secondary">
          Turn ModSecurity on or off for your websites, and disable a rule when it blocks a real request.
        </Typography.Paragraph>
      </div>
      <AccountWAF />
    </div>
  );
}
