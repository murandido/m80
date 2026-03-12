import type { NextConfig } from "next"
import { envSchema } from "@/lib/env"

const nextConfig: NextConfig = {
  /* config options here */
  reactCompiler: true,
}

// validate the env scheme when building
envSchema.parse(process.env)

export default nextConfig
