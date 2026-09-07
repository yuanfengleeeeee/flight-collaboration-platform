export {};

declare global {
  const wx: {
    login(options: {
      success: (result: { code?: string; errMsg?: string }) => void;
      fail: (error: unknown) => void;
    }): void;
    qy?: {
      login(options: {
        success: (result: { code?: string; errMsg?: string }) => void;
        fail: (error: unknown) => void;
      }): void;
    };
    request(options: {
      url: string;
      method?: "GET" | "POST";
      data?: unknown;
      header?: Record<string, string>;
      success: (response: { statusCode: number; data: unknown }) => void;
      fail: (error: unknown) => void;
    }): void;
    connectSocket(options: {
      url: string;
      protocols?: string[];
      header?: Record<string, string>;
      success?: () => void;
      fail?: (error: unknown) => void;
    }): MiniappSocketTask;
    getStorageSync(key: string): unknown;
    setStorageSync(key: string, value: unknown): void;
    removeStorageSync(key: string): void;
    reLaunch(options: { url: string }): void;
    navigateTo(options: { url: string }): void;
  };

  function App<T extends Record<string, unknown>>(definition: T): void;
  type MiniappPageContext<TData extends object> = { setData(data: Partial<TData>): void };
  function Page<T extends { data: object }>(definition: T & ThisType<T & MiniappPageContext<T["data"]>>): void;
}

declare global {
  interface MiniappSocketTask {
    onOpen(callback: () => void): void;
    onMessage(callback: (event: { data: unknown }) => void): void;
    onError(callback: (error: unknown) => void): void;
    onClose(callback: (event: unknown) => void): void;
    send(options: { data: string; success?: () => void; fail?: (error: unknown) => void }): void;
    close(options?: { code?: number; reason?: string }): void;
  }
}
