import { Card, Typography } from "antd";
import { Mark } from "@/components/Mark";

export function SetupLocked({ invalidToken }: { invalidToken: boolean }) {
  return (
    <div className="siroc-login">
      <Card className="siroc-login-card" style={{ maxWidth: 480 }}>
        <div className="siroc-login-brand">
          <Mark />
          <Typography.Title level={2}>Siroc</Typography.Title>
        </div>
        <p className="siroc-login-note">First-run setup</p>
        {invalidToken ? (
          <Typography.Paragraph type="danger">
            This setup link is invalid or has already been used.
          </Typography.Paragraph>
        ) : (
          <Typography.Paragraph type="secondary">
            This panel has no administrator yet. Open the one-time setup link printed by the
            installer. It includes a secret token and is the only way to create the first admin.
          </Typography.Paragraph>
        )}
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          On the server, the link is also saved at <code>/var/lib/siroc/setup.url</code> until
          the first admin is created.
        </Typography.Paragraph>
      </Card>
    </div>
  );
}
