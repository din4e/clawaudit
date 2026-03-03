/** @type {import('next').NextConfig} */
const nextConfig = {
  // Disable server-based features for static export
  output: 'export',
  // Disable image optimization for static export
  images: {
    unoptimized: true,
  },
  // Trailing slash for static hosting compatibility
  trailingSlash: true,
  // Clean dist folder
  distDir: 'out',
};

module.exports = nextConfig;
