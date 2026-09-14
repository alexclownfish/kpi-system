import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  output: process.env.NEXT_OUTPUT_MODE === "standalone" ? "standalone" : undefined,
  basePath: process.env.NEXT_PUBLIC_BASE_PATH || '',
};

export default nextConfig;
