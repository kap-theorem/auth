import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: 'standalone',
  serverExternalPackages: ['@grpc/grpc-js', '@grpc/proto-loader'],
  turbopack: {},
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: [
          {
            key: 'Content-Security-Policy',
            value: "frame-ancestors 'self' https://*.kaplabs.dev http://*.kaplabs.dev",
          },
          {
            key: 'X-Frame-Options',
            value: 'ALLOW-FROM https://*.kaplabs.dev http://*.kaplabs.dev',
          },
        ],
      },
    ];
  },
  webpack: (config, { isServer }) => {
    if (isServer) {
      // next.js standalone mode doesn't copy non-JS files by default.
      // We instruct webpack to ignore the dynamic require warnings for @grpc
      config.ignoreWarnings = [
        { module: /node_modules\/@grpc/ }
      ];
    }
    return config;
  }
};

export default nextConfig;
