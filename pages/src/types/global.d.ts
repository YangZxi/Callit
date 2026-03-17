export {};

declare global {
  interface Window {
    __BASE_PREFIX__: string;
  }
}