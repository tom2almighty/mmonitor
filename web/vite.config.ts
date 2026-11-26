import fs from "fs"
import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type Plugin } from "vite"

const logoSvgPath = path.resolve(__dirname, "../assets/logo.svg")
const distGitkeepPath = path.resolve(__dirname, "./dist/.gitkeep")

function mmonitorAssetsPlugin(): Plugin {
  return {
    name: "mmonitor-assets",
    configureServer(server) {
      server.middlewares.use("/favicon.svg", (_req, res, next) => {
        if (!fs.existsSync(logoSvgPath)) {
          next()
          return
        }
        res.setHeader("Content-Type", "image/svg+xml; charset=utf-8")
        res.end(fs.readFileSync(logoSvgPath))
      })
    },
    closeBundle() {
      fs.mkdirSync(path.dirname(distGitkeepPath), { recursive: true })
      if (!fs.existsSync(distGitkeepPath)) {
        fs.writeFileSync(distGitkeepPath, "")
      }
    },
  }
}

export default defineConfig({
  publicDir: false,
  plugins: [react(), tailwindcss(), mmonitorAssetsPlugin()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/health": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/LICENSE": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
})
