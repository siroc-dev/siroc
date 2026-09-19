import { Card, Typography } from "antd";

export function SetupLocked({ invalidToken }: { invalidToken: boolean }) {
  return (
    <div style={{ display: "grid", placeItems: "center", minHeight: "100vh", padding: 24 }}>
      <Card style={{ width: "100%", maxWidth: 480 }}>
        <Typography.Title level={3} style={{ marginTop: 0 }}>
          First-run setup
        </Typography.Title>
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
