export type FrontendMode = "local-api" | "mock";

export function frontendMode(): FrontendMode {
  const env = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env;
  return env?.VITE_FRONTEND_MODE === "mock" ? "mock" : "local-api";
}
