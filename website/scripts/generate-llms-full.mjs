// Generates llms-full.txt by concatenating all doc pages into a single
// Markdown file, stripping frontmatter and MDX/Starlight component syntax.

import { readFileSync, writeFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const docsDir = resolve(__dirname, "../src/content/docs");
const outFile = resolve(__dirname, "../public/llms-full.txt");

// Pages in sidebar order (matching astro.config.mjs), skipping index.mdx splash page.
const pages = [
  "overview.md",
  "installation.md",
  "commands.md",
  "guides/lifecycle-hooks.md",
  "guides/smart-restart.md",
  "guides/git-integration.mdx",
  "guides/custom-config.md",
  "reference/troubleshooting.md",
  "reference/changelog.md",
  "contributing/development.md",
  "contributing/roadmap.md",
  "contributing/implementation-notes.md",
  "contributing/devcontainers-spec.md",
];

function extractFrontmatter(content) {
  const match = content.match(/^---\n([\s\S]*?)\n---\n/);
  if (!match) return { title: "Untitled", body: content };

  const fm = match[1];
  const titleMatch = fm.match(/^title:\s*(.+)$/m);
  const title = titleMatch ? titleMatch[1].trim() : "Untitled";
  const body = content.slice(match[0].length);
  return { title, body };
}

function cleanMdx(text) {
  let out = text;

  // Remove import statements.
  out = out.replace(/^import\s+.*;\s*\n/gm, "");

  // Convert <Tabs>/<TabItem> blocks: extract content, dedent, and add a bold label.
  out = out.replace(
    /<TabItem\s+label="([^"]+)">\s*\n([\s\S]*?)<\/TabItem>/g,
    (_, label, content) => {
      const dedented = content.replace(/^    /gm, "");
      return `**${label}:**\n\n${dedented}`;
    },
  );
  out = out.replace(/<Tabs[^>]*>\s*\n?/g, "");
  out = out.replace(/<\/Tabs>\s*\n?/g, "");

  // Strip <CardGrid> / </CardGrid>.
  out = out.replace(/<CardGrid>\s*\n?/g, "");
  out = out.replace(/<\/CardGrid>\s*\n?/g, "");

  // Convert <Card title="X" ...> to bold heading, strip </Card>.
  out = out.replace(/<Card\s+title="([^"]+)"[^>]*>\s*\n?/g, "**$1:** ");
  out = out.replace(/<\/Card>\s*\n?/g, "\n");

  // Convert :::note[text] / :::caution / :::tip callouts to blockquotes.
  out = out.replace(/^:::(\w+)\[([^\]]+)\]\s*$/gm, "> **$2**");
  out = out.replace(/^:::(\w+)\s*$/gm, (_, type) => {
    const label = type.charAt(0).toUpperCase() + type.slice(1);
    // Closing ::: has no type keyword, handled below.
    if (type === "") return "";
    return `> **${label}:**`;
  });

  // Convert content lines between callout markers to blockquotes.
  // This is a simplified approach: lines after a "> **" line until ":::" get prefixed with ">".
  const lines = out.split("\n");
  const result = [];
  let inCallout = false;
  for (const line of lines) {
    if (line.startsWith("> **") && !inCallout) {
      inCallout = true;
      result.push(line);
    } else if (inCallout && line.trim() === ":::") {
      inCallout = false;
    } else if (inCallout) {
      result.push(line === "" ? ">" : `> ${line}`);
    } else {
      result.push(line);
    }
  }
  out = result.join("\n");

  // Collapse 3+ consecutive blank lines into 2.
  out = out.replace(/\n{3,}/g, "\n\n");

  return out.trim();
}

const header = `# crib

> Dev containers without the ceremony. A CLI tool that reads .devcontainer configs, builds the container, and gets out of the way. Supports Docker and Podman.

Full documentation: https://fgrehm.github.io/crib/
Source code: https://github.com/fgrehm/crib`;

const sections = pages.map((slug) => {
  const filePath = resolve(docsDir, slug);
  const raw = readFileSync(filePath, "utf-8");
  const { title, body } = extractFrontmatter(raw);
  const cleaned = cleanMdx(body);
  return `## ${title}\n\n${cleaned}`;
});

const output = [header, ...sections].join("\n\n---\n\n") + "\n";

writeFileSync(outFile, output);
console.log(`Generated ${outFile} (${pages.length} pages)`);
