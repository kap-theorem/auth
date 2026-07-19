import { MockApiClient } from "./mock";

// Single shared client instance. When the grpc-gateway client exists,
// construct it here instead; every page consumes only the ApiClient interface
// (plus MockApiClient's signOut/restoreSession session helpers, which the
// real client will also provide).
export const api = new MockApiClient();

export * from "./types";
