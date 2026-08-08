import type { NextConfig } from "next";

const controlPlaneURL = process.env.HUBCR_CONTROL_PLANE_URL ?? "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${controlPlaneURL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
