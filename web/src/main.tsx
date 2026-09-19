import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App as AntdApp, ConfigProvider } from "antd";
import { App } from "./App";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: "#4f46e5",
          borderRadius: 8,
          fontFamily: "Inter, system-ui, -apple-system, Segoe UI, sans-serif",
        },
        components: {
          Layout: {
            siderBg: "#1e1b4b",
            headerBg: "#fff",
          },
          Menu: {
            darkItemBg: "#1e1b4b",
            darkSubMenuItemBg: "#1e1b4b",
            darkItemSelectedBg: "#4338ca",
            darkItemHoverBg: "#312e81",
          },
        },
      }}
    >
      <AntdApp>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </AntdApp>
    </ConfigProvider>
  </StrictMode>,
);
