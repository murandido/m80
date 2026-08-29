import type { NextConfig } from "next"
import { envSchema } from "@/lib/env"

const nextConfig: NextConfig = {
  /* config options here */
  // Vercel packaged the app for us; self-hosted, this is what keeps the image
  // from having to ship 800 MB of node_modules just to run `next start`.
  output: "standalone",
  reactCompiler: true,
  experimental: {
    serverActions: {
      // The spreadsheet importer uploads the whole workbook through a server action, and
      // the default ceiling is 1 MB - a month of the real file is already ~600 KB, and a
      // fuller one would be refused with a message about nothing in particular. Raise it
      // back to the default when that feature is removed.
      bodySizeLimit: "10mb",
    },
  },
}

// validate the env scheme when building
envSchema.parse(process.env)

export default nextConfig
