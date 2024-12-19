import path from "node:path";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "backend.tarhche.com",
      },
    ],
  },
  experimental: {
    optimizePackageImports: [
      "@mantine/core",
      "@mantine/dates",
      "@mantine/hooks",
      "@mantine/notifications",
      "@mantine/tiptap",
      "@mantine/code-highlight",
    ],
  },
  webpack: (config, {isServer}) => {
    if (!isServer) {
      config.resolve.alias["yjs"] = path.resolve(
        import.meta.dirname,
        "node_modules/yjs",
      );
    }
    return config;
  },
};

export default nextConfig;
