/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** "real" to use the backend on :8081; defaults to the in-memory mock. */
  readonly VITE_API_MODE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
