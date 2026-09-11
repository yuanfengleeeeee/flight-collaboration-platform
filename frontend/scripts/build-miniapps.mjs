import { build } from "esbuild";
import { cpSync, mkdirSync, readdirSync, statSync } from "node:fs";
import { dirname, extname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const releaseBuild = process.env.MINIAPP_RELEASE === "true";
const edgeBaseUrl = (process.env.MINIAPP_EDGE_API_BASE_URL ?? "https://edge.example.invalid").trim();

if (releaseBuild && isPlaceholderURL(edgeBaseUrl)) {
  throw new Error("MINIAPP_RELEASE=true 时必须提供真实 HTTPS MINIAPP_EDGE_API_BASE_URL");
}

const apps = [
  { name: "employee-miniapp", label: "个人微信" },
  { name: "employee-wecom-miniapp", label: "企业微信" },
];

for (const app of apps) {
  const sourceRoot = resolve(frontendRoot, "apps", app.name);
  const outputRoot = resolve(frontendRoot, "dist", app.name);
  mkdirSync(outputRoot, { recursive: true });

  for (const file of findFiles(sourceRoot)) {
    const relativePath = relative(sourceRoot, file).split("\\").join("/");
    if (!isStaticAsset(relativePath)) continue;
    const destination = resolve(outputRoot, relativePath);
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(file, destination);
  }

  const entries = [
    resolve(sourceRoot, "app.ts"),
    ...findFiles(resolve(sourceRoot, "pages")).filter((file) => extname(file) === ".ts"),
  ];
  for (const entry of entries) {
    const relativeEntry = relative(sourceRoot, entry);
    const outputFile = resolve(outputRoot, relativeEntry.replace(/\.ts$/u, ".js"));
    mkdirSync(dirname(outputFile), { recursive: true });
    await build({
      absWorkingDir: frontendRoot,
      entryPoints: [entry],
      outfile: outputFile,
      bundle: true,
      charset: "utf8",
      define: { MINIAPP_EDGE_API_BASE_URL: JSON.stringify(edgeBaseUrl) },
      format: "iife",
      logLevel: "warning",
      minify: releaseBuild,
      platform: "browser",
      sourcemap: !releaseBuild,
      target: "es2018",
    });
  }

  console.log(`${app.label}小程序已生成：${relative(frontendRoot, outputRoot)}，Edge：${edgeBaseUrl}`);
}

function findFiles(directory) {
  if (!statExists(directory)) return [];
  const files = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const absolutePath = resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...findFiles(absolutePath));
    else files.push(absolutePath);
  }
  return files;
}

function isStaticAsset(relativePath) {
  if (["app.json", "app.wxss", "project.config.json", "sitemap.json"].includes(relativePath)) return true;
  return relativePath.startsWith("pages/") && [".json", ".wxml", ".wxss"].includes(extname(relativePath));
}

function isPlaceholderURL(value) {
  return !/^https:\/\/[^/]+/u.test(value) || /(?:example\.invalid|localhost|127\.0\.0\.1)/iu.test(value);
}

function statExists(file) {
  try {
    statSync(file);
    return true;
  } catch {
    return false;
  }
}
