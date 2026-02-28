import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://fgrehm.github.io",
  base: "/crib",
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
            { label: "Changelog", slug: "reference/changelog" },
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
              label: "DevContainer Spec Reference",
              slug: "contributing/devcontainers-spec",
            },
            {
              label: "Remote Features Design",
              slug: "contributing/remote-features-design",
            },
            {
              label: "Extensions Vision",
              slug: "contributing/crib-extensions",
            },
          ],
        },
      ],
    }),
  ],
});
