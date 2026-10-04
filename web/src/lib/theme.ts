export type ThemeStyle = "auto" | "light" | "dark";
export type ThemeColor = "default" | "mint" | "violet" | "sky" | "sakura" | "blackgold" | "custom";

export type Brand = {
  themeStyle: ThemeStyle;
  themeColor: ThemeColor;
  themeCustom?: string;
  logoUrl?: string;
  faviconUrl?: string;
  hasLogo?: boolean;
  hasFavicon?: boolean;
};

export type ColorPreset = {
  id: ThemeColor;
  label: string;
  primary: string;
  sider: string;
  selected: string;
  hover: string;
};

export const DEFAULT_BRAND: Brand = {
  themeStyle: "dark",
  themeColor: "default",
};

export const THEME_COLORS: ColorPreset[] = [
  { id: "default", label: "Siroc", primary: "#e2783a", sider: "#0c0b0a", selected: "#3d2918", hover: "#1c1814" },
  { id: "mint", label: "Mint", primary: "#10b981", sider: "#064e3b", selected: "#059669", hover: "#065f46" },
  { id: "violet", label: "Violet", primary: "#7c3aed", sider: "#2e1065", selected: "#6d28d9", hover: "#4c1d95" },
  { id: "sky", label: "Sky blue", primary: "#0ea5e9", sider: "#0c4a6e", selected: "#0284c7", hover: "#075985" },
  { id: "sakura", label: "Sakura", primary: "#ec4899", sider: "#831843", selected: "#db2777", hover: "#9d174d" },
  { id: "blackgold", label: "Black gold", primary: "#d4a017", sider: "#1c1917", selected: "#b45309", hover: "#292524" },
  { id: "custom", label: "Custom", primary: "#4f46e5", sider: "#1e1b4b", selected: "#4338ca", hover: "#312e81" },
];

export function colorPreset(color: ThemeColor | string | undefined, custom?: string): ColorPreset {
  const found = THEME_COLORS.find((c) => c.id === color) || THEME_COLORS[0];
  if (found.id !== "custom") return found;
  const primary = /^#[0-9a-fA-F]{6}$/.test(custom || "") ? (custom as string) : found.primary;
  return { ...found, primary };
}

export function resolveDark(style: ThemeStyle | string | undefined, prefersDark: boolean) {
  if (style === "dark") return true;
  if (style === "light") return false;
  return prefersDark;
}
