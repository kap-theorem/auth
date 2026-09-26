import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { api, DeveloperSession, PLATFORM_CLIENT_ID } from "./api";

interface AuthState {
  session: DeveloperSession | null;
  isSuperadmin: boolean;
  ready: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  signOut: () => void;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<DeveloperSession | null>(null);
  const [isSuperadmin, setIsSuperadmin] = useState(false);
  const [ready, setReady] = useState(false);

  // Spec §8: one login, two audiences. After login ask
  // Check(platform, me, admin, app:platform) and gate superadmin nav on allow.
  const adopt = useCallback(async (s: DeveloperSession) => {
    const res = await api.check(
      PLATFORM_CLIENT_ID,
      `user:${s.developer.developer_id}`,
      "admin",
      "app:platform"
    );
    setSession(s);
    setIsSuperadmin(res.allowed);
  }, []);

  useEffect(() => {
    const restored = api.restoreSession();
    if (restored) {
      adopt(restored).finally(() => setReady(true));
    } else {
      setReady(true);
    }
  }, [adopt]);

  const signIn = useCallback(
    async (email: string, password: string) => adopt(await api.developerLogin(email, password)),
    [adopt]
  );

  const register = useCallback(
    async (email: string, password: string) => adopt(await api.registerDeveloper(email, password)),
    [adopt]
  );

  const signOut = useCallback(() => {
    api.signOut();
    setSession(null);
    setIsSuperadmin(false);
  }, []);

  return (
    <AuthContext.Provider value={{ session, isSuperadmin, ready, signIn, register, signOut }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
