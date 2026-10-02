import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ConfigProvider, theme } from "antd";
import { api } from "@/lib/api";
import { colorPreset, DEFAULT_BRAND, resolveDark, type Brand } from "@/lib/theme";

type BrandCtx = {
  brand: Brand;
  setBrand: (next: Brand) => void;
  refresh: () => Promise<void>;
  resolvedDark: boolean;
};

const BrandContext = createContext<BrandCtx>({
  brand: DEFAULT_BRAND,
  setBrand: () => undefined,
  refresh: async () => undefined,
  resolvedDark: false,
});

export function useBrand() {
  return useContext(BrandContext);
}

function prefersDark() {
  return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [brand, setBrand] = useState<Brand>(DEFAULT_BRAND);
  const [darkPref, setDarkPref] = useState(prefersDark);

  async function refresh() {
    try {
      const next = await api.get<Brand>("/api/brand");
      setBrand({ ...DEFAULT_BRAND, ...next });
    } catch {
      /* keep last known */
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined);
  }, []);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setDarkPref(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const resolvedDark = resolveDark(brand.themeStyle, darkPref);
  const preset = colorPreset(brand.themeColor, brand.themeCustom);

  useEffect(() => {
    document.documentElement.dataset.theme = resolvedDark ? "dark" : "light";
  }, [resolvedDark]);

  useEffect(() => {
    let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (!brand.faviconUrl) return;
    if (!link) {
      link = document.createElement("link");
      link.rel = "icon";
      document.head.appendChild(link);
    }
    link.href = brand.faviconUrl;
  }, [brand.faviconUrl]);

  const antdTheme = useMemo(
    () => ({
      algorithm: resolvedDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
      token: {
        colorPrimary: preset.primary,
        borderRadius: 8,
        fontFamily: "Inter, system-ui, -apple-system, Segoe UI, sans-serif",
      },
      components: {
        Layout: {
          siderBg: preset.sider,
          headerBg: resolvedDark ? "#141414" : "#fff",
        },
        Menu: {
          darkItemBg: preset.sider,
          darkSubMenuItemBg: preset.sider,
          darkItemSelectedBg: preset.selected,
          darkItemHoverBg: preset.hover,
        },
      },
    }),
    [preset, resolvedDark],
  );

  return (
    <BrandContext.Provider value={{ brand, setBrand, refresh, resolvedDark }}>
      <ConfigProvider theme={antdTheme}>{children}</ConfigProvider>
    </BrandContext.Provider>
  );
}
