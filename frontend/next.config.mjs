/** @type {import('next').NextConfig} */

const nextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "backend-reactjs.tarhche.com",
      },
    ],
  },
  experimental: {},
};

export default nextConfig;
