import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import rehypeExternalLinks from "rehype-external-links";

export default defineConfig({
  site: "https://fgrehm.github.io",
  base: "/crib",
  markdown: {
    rehypePlugins: [
      [
        rehypeExternalLinks,
        {
          target: "_blank",
          rel: ["noopener", "noreferrer"],
        },
      ],
    ],
  },
  vite: {
    resolve: {
      preserveSymlinks: true,
    },
  },
  integrations: [
    starlight({
      title: "crib docs",
      favicon: "/favicon.svg",
      logo: {
        light: "./src/assets/logo-light.svg",
        dark: "./src/assets/logo-dark.svg",
        alt: "crib logo",
        replacesTitle: true,
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/fgrehm/crib",
        },
      ],
      editLink: {
        baseUrl: "https://github.com/fgrehm/crib/edit/main/website/",
      },
      head: [
        {
          tag: "link",
          attrs: {
            rel: "alternate",
            type: "text/plain",
            href: "/crib/llms.txt",
            title: "LLM-friendly documentation index",
          },
        },
        {
          tag: "link",
          attrs: {
            rel: "alternate",
            type: "text/plain",
            href: "/crib/llms-full.txt",
            title: "LLM-friendly full documentation",
          },
        },
      ],
      customCss: ["./src/styles/custom.css"],
      sidebar: [
        {
          label: "Getting Started",
          items: [
            { label: "Overview", slug: "overview" },
            { label: "Installation", slug: "installation" },
            { label: "Commands", slug: "commands" },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "Lifecycle Hooks", slug: "guides/lifecycle-hooks" },
            { label: "Smart Restart", slug: "guides/smart-restart" },
            { label: "Git Integration", slug: "guides/git-integration" },
            {
              label: "Custom Config Directory",
              slug: "guides/custom-config",
            },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "Troubleshooting", slug: "reference/troubleshooting" },
            { label: "CHANGELOG", slug: "reference/changelog" },
          ],
        },
        {
          label: "Contributing",
          items: [
            { label: "Development", slug: "contributing/development" },
            { label: "Roadmap", slug: "contributing/roadmap" },
            {
              label: "Implementation Notes",
              slug: "contributing/implementation-notes",
            },
            {
              label: "Plugin Development",
              slug: "contributing/plugin-development",
            },
            {
              label: "DevContainer Spec Reference",
              slug: "contributing/devcontainers-spec",
            },
          ],
        },
      ],
    }),
  ],
});
