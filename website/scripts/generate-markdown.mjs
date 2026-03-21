// Generates:
//   - public/{slug}.md         — individual cleaned Markdown files for each page
//   - public/llms-full.txt     — all pages concatenated into one file
//   - public/llms.txt          — index with links to the .md files
//
// Strips frontmatter and MDX/Starlight syntax so the output is plain Markdown.

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const docsDir = resolve(__dirname, "../src/content/docs");
const publicDir = resolve(__dirname, "../public");
const fullOutFile = resolve(publicDir, "llms-full.txt");
const indexOutFile = resolve(publicDir, "llms.txt");

const baseUrl = "https://fgrehm.github.io/crib";

// Pages in sidebar order (matching astro.config.mjs), skipping index.mdx splash page.
const pages = [
  "overview.md",
  "installation.md",
  "commands.md",
  "examples.md",
  "comparison.md",
  "guides/workspaces.md",
  "guides/lifecycle-hooks.md",
  "guides/smart-restart.md",
  "guides/plugins.md",
  "guides/custom-config.md",
  "guides/macos-windows.md",
  "reference/commands.md",
  "reference/troubleshooting.md",
  "reference/changelog.md",
  "contributing/development.md",
  "contributing/roadmap.md",
  "contributing/implementation-notes.md",
  "contributing/plugin-development.md",
  "contributing/devcontainers-spec.md",
];

// Section labels keyed by directory prefix.
const sectionLabels = {
  "": "Getting Started",
  "guides": "Guides",
  "reference": "Reference",
  "contributing": "Contributing",
};

function extractFrontmatter(content) {
  const match = content.match(/^---\n([\s\S]*?)\n---\n/);
  if (!match) return { title: "Untitled", description: "", body: content };

  const fm = match[1];
  const titleMatch = fm.match(/^title:\s*(.+)$/m);
  const descMatch = fm.match(/^description:\s*(.+)$/m);
  const title = titleMatch ? titleMatch[1].trim() : "Untitled";
  const description = descMatch ? descMatch[1].trim() : "";
  const body = content.slice(match[0].length);
  return { title, description, body };
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

// Derive a URL slug from a page filename.
function pageSlug(page) {
  return page.replace(/\.mdx?$/, "").replace(/\/$/, "");
}

// --- Read and clean all pages ---

const pageMeta = pages.map((slug) => {
  const filePath = resolve(docsDir, slug);
  const raw = readFileSync(filePath, "utf-8");
  const { title, description, body } = extractFrontmatter(raw);
  const cleaned = cleanMdx(body);
  return { slug, title, description, cleaned };
});

// --- Generate individual .md files ---

let mdCount = 0;
for (const { slug, title, cleaned } of pageMeta) {
  const outPath = resolve(publicDir, slug);
  mkdirSync(dirname(outPath), { recursive: true });
  const content = `# ${title}\n\n${cleaned}\n`;
  writeFileSync(outPath, content);
  mdCount++;
}
console.log(`Generated ${mdCount} individual .md files in public/`);

// --- Generate llms-full.txt ---

const header = `# crib

> Just Enough Devcontainers. A CLI tool that reads .devcontainer configs, builds the container, and gets out of the way. Supports Docker and Podman.

Full documentation: ${baseUrl}/
Source code: https://github.com/fgrehm/crib`;

const sections = pageMeta.map(({ title, cleaned }) => {
  return `## ${title}\n\n${cleaned}`;
});

const fullOutput = [header, ...sections].join("\n\n---\n\n") + "\n";
writeFileSync(fullOutFile, fullOutput);
console.log(`Generated ${fullOutFile} (${pages.length} pages)`);

// --- Generate llms.txt ---

const indexHeader = `# crib

> Just Enough Devcontainers. A CLI tool that reads .devcontainer configs, builds the container, and gets out of the way. Supports Docker and Podman.

crib is a devcontainer CLI tool. It reads \`.devcontainer/devcontainer.json\` configs, builds the container (image-based, Dockerfile-based, or Docker Compose-based), and manages the full lifecycle: creating, starting, stopping, rebuilding, and removing dev containers. No agents, no SSH, no IDE integration, just the CLI.

For the full documentation in a single file, see [llms-full.txt](${baseUrl}/llms-full.txt).`;

// Group pages by section.
let currentSection = "";
const indexLines = [];

for (const { slug, title, description } of pageMeta) {
  const dir = slug.includes("/") ? slug.split("/")[0] : "";
  const label = sectionLabels[dir] || dir;

  if (label !== currentSection) {
    currentSection = label;
    indexLines.push(`\n## ${label}\n`);
  }

  const mdUrl = `${baseUrl}/${pageSlug(slug)}.md`;
  const desc = description ? `: ${description}` : "";
  indexLines.push(`- [${title}](${mdUrl})${desc}`);
}

indexLines.push(`\n## Source Code\n`);
indexLines.push(`- [GitHub Repository](https://github.com/fgrehm/crib)`);

const indexOutput = indexHeader + "\n" + indexLines.join("\n") + "\n";
writeFileSync(indexOutFile, indexOutput);
console.log(`Generated ${indexOutFile} (${pages.length} pages)`);
