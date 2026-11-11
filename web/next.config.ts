import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // a self-contained server in .next/standalone, used by the Docker image
  output: "standalone",
};

export default nextConfig;
