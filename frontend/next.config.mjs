/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      {
        protocol: process.env.NEXT_PUBLIC_FILES_PROTOCOL,
        hostname: process.env.NEXT_PUBLIC_FILES_HOST,
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
};

export default nextConfig;
