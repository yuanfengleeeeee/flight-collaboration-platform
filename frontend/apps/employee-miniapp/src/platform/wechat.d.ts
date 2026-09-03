declare const wx: {
  login(options: {
    success: (result: { code?: string; errMsg?: string }) => void;
    fail: (error: unknown) => void;
  }): void;
  request(options: {
    url: string;
    method?: "GET" | "POST";
    data?: unknown;
    header?: Record<string, string>;
    success: (response: { statusCode: number; data: unknown }) => void;
    fail: (error: unknown) => void;
  }): void;
  getStorageSync(key: string): unknown;
  setStorageSync(key: string, value: unknown): void;
  removeStorageSync(key: string): void;
  reLaunch(options: { url: string }): void;
  navigateTo(options: { url: string }): void;
};

declare function App<T extends Record<string, unknown>>(definition: T): void;
type MiniappPageContext<TData extends object> = { setData(data: Partial<TData>): void };
declare function Page<T extends { data: object }>(definition: T & ThisType<T & MiniappPageContext<T["data"]>>): void;
