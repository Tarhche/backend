import path from "node:path";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      {
        protocol: process.env.NEXT_PUBLIC_FILES_PROTOCOL,
        hostname: process.env.NEXT_PUBLIC_FILES_HOST,
        port: process.env.NEXT_PUBLIC_FILES_PORT,
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
