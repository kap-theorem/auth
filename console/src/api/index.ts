import { MockApiClient } from "./mock";
import { RealApiClient } from "./real";
import type { ApiClient } from "./types";

// Single swap point (spec §8). Selection is build/env driven:
//   VITE_API_MODE=real  → RealApiClient (backend on :8081, via the vite proxy)
//   anything else       → MockApiClient (in-memory, default)
// Every page consumes only the ApiClient interface.
export type ApiMode = "mock" | "real";

export const apiMode: ApiMode = import.meta.env.VITE_API_MODE === "real" ? "real" : "mock";

export const api: ApiClient = apiMode === "real" ? new RealApiClient() : new MockApiClient();

export * from "./types";
