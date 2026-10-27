import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  // The frontend runs inside the system webview, whose JavaScript engine
  // varies by platform. Targeting ES2020 keeps the bundle within what
  // every current WebKit and WebView2 build supports.
  build: {
    target: "es2020",
  },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [wails("./bindings")],
});
