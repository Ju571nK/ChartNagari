/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_DEMO_STATIC?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
