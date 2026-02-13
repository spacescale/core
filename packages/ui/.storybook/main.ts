import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: [
    "@storybook/addon-essentials",
    "@storybook/addon-themes",
  ],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  viteFinal: async (config) => {
    const { default: tailwindcss } = await import("tailwindcss");
    const { default: autoprefixer } = await import("autoprefixer");

    config.css = {
      postcss: {
        plugins: [tailwindcss(), autoprefixer()],
      },
    };

    // Add path alias for @/
    config.resolve = {
      ...config.resolve,
      alias: {
        ...config.resolve?.alias,
        "@": new URL("../src", import.meta.url).pathname,
      },
    };

    return config;
  },
};

export default config;
