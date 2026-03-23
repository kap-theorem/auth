"use client";

import { useState, useEffect, Suspense, use } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

interface PageProps {
    params: Promise<{ clientId: string }>;
}

function RegisterForm({ clientId }: { clientId: string }) {
    const router = useRouter();
    const searchParams = useSearchParams();
    const inviteToken = searchParams.get("invite_token");
    const prefilledEmail = searchParams.get("email");
    const redirectUrl = searchParams.get("redirect") || "/dashboard";

    const [username, setUsername] = useState("");
    const [email, setEmail] = useState(prefilledEmail || "");
    const [password, setPassword] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [configLoading, setConfigLoading] = useState(true);
    const [isInviteOnly, setIsInviteOnly] = useState(false);
    const [clientName, setClientName] = useState("");

    useEffect(() => {
        fetch(`/api/client/${clientId}/config`)
            .then(res => res.json())
            .then(data => {
                if (data.success && data.config) {
                    setIsInviteOnly(data.config.invite_only || false);
                }
                if (data.success && data.client_name) {
                    setClientName(data.client_name);
                }
            })
            .catch(() => {})
            .finally(() => setConfigLoading(false));
    }, [clientId]);

    const hasValidInvite = isInviteOnly ? !!inviteToken : true;

    const handleRegister = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError("");

        try {
            const res = await fetch("/api/auth/register", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ username, email, password, clientId, inviteToken }),
            });

            const data = await res.json();

            if (data.success) {
                router.push(`/client/${clientId}/user/login?redirect=${encodeURIComponent(redirectUrl)}&registered=true`);
            } else {
                setError(data.message || "Failed to create account");
            }
        } catch (err) {
            setError("Failed to connect to authentication server.");
        } finally {
            setLoading(false);
        }
    };

    if (configLoading) {
        return <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading...</div>;
    }

    return (
        <div className="relative min-h-screen flex items-center justify-center p-4 overflow-hidden">
            {/* Dynamic Background Elements */}
            <div className="absolute inset-0 z-0">
                <div className="absolute top-1/4 left-1/4 w-72 h-72 bg-pink-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob" />
                <div className="absolute bottom-1/4 right-1/4 w-72 h-72 bg-blue-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob animation-delay-4000" />
            </div>

            <div className="z-10 w-full max-w-md">
                <div className="glass-card rounded-2xl p-8 shadow-2xl relative overflow-hidden">
                    {/* Subtle glow effect top border */}
                    <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-pink-500 via-purple-500 to-blue-500 opacity-80"></div>

                    <div className="text-center mb-8">
                        <h1 className="text-3xl font-bold tracking-tight text-zinc-900 dark:text-white mb-2">
                            {clientName ? `Join ${clientName}` : "Create an account"}
                        </h1>
                        <p className="text-sm text-zinc-500 dark:text-zinc-400">
                            Authentication powered by auth.kaplabs.dev
                        </p>
                    </div>

                    {error && (
                        <div className="mb-6 p-3 rounded-lg bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 text-red-600 dark:text-red-400 text-sm font-medium text-center">
                            {error}
                        </div>
                    )}

                    {!hasValidInvite ? (
                        <div className="text-center py-6 px-4 bg-zinc-50 dark:bg-black/20 border border-zinc-200 dark:border-white/10 rounded-xl space-y-4">
                            <svg className="w-12 h-12 mx-auto text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>
                            <div>
                                <h3 className="text-lg font-bold text-zinc-900 dark:text-white mb-1">Invite Required</h3>
                                <p className="text-sm text-zinc-600 dark:text-zinc-400">
                                    This is currently an invite-only application.
                                    You need an invite link from an administrator to create an account.
                                </p>
                            </div>
                            <div className="pt-4">
                                <Link
                                    href={`/client/${clientId}/user/login?redirect=${encodeURIComponent(redirectUrl)}`}
                                    className="px-6 py-2.5 bg-zinc-900 dark:bg-white text-white dark:text-black rounded-lg text-sm font-semibold hover:opacity-90 transition-opacity inline-block"
                                >
                                    Return to Sign In
                                </Link>
                            </div>
                        </div>
                    ) : (
                        <>
                            {inviteToken && (
                                <div className="mb-6 py-2 px-3 bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-200 dark:border-emerald-500/20 text-emerald-800 dark:text-emerald-400 rounded-lg text-xs font-semibold flex items-center justify-center gap-2">
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                                    Valid invite token attached
                                </div>
                            )}

                            <form onSubmit={handleRegister} className="space-y-4">
                                <div>
                                    <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                                        Username
                                    </label>
                                    <input
                                        type="text"
                                        required
                                        value={username}
                                        onChange={(e) => setUsername(e.target.value)}
                                        className="w-full px-4 py-3 rounded-xl bg-white/50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-purple-500 focus:border-transparent outline-none transition-all dark:text-white"
                                        placeholder="johndoe123"
                                    />
                                </div>
                                <div>
                                    <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                                        Email Address
                                    </label>
                                    <input
                                        type="email"
                                        required
                                        value={email}
                                        onChange={(e) => setEmail(e.target.value)}
                                        className="w-full px-4 py-3 rounded-xl bg-white/50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-purple-500 focus:border-transparent outline-none transition-all dark:text-white"
                                        placeholder="you@example.com"
                                    />
                                </div>
                                <div>
                                    <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                                        Password
                                    </label>
                                    <input
                                        type="password"
                                        required
                                        value={password}
                                        onChange={(e) => setPassword(e.target.value)}
                                        className="w-full px-4 py-3 rounded-xl bg-white/50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-purple-500 focus:border-transparent outline-none transition-all dark:text-white"
                                        placeholder="••••••••"
                                    />
                                </div>

                                <button
                                    type="submit"
                                    disabled={loading}
                                    className="mt-6 w-full py-3 px-4 rounded-xl bg-zinc-900 dark:bg-white text-white dark:text-black font-semibold shadow-lg hover:shadow-xl hover:-translate-y-0.5 transition-all duration-200 disabled:opacity-70 disabled:hover:translate-y-0 flex justify-center items-center"
                                >
                                    {loading ? (
                                        <svg className="animate-spin h-5 w-5 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                        </svg>
                                    ) : (
                                        "Create Account"
                                    )}
                                </button>
                            </form>
                        </>
                    )}

                    <p className="mt-8 text-center text-sm text-zinc-500 dark:text-zinc-400">
                        Already have an account?{" "}
                        <Link
                            href={`/client/${clientId}/user/login?redirect=${encodeURIComponent(redirectUrl)}`}
                            className="font-semibold text-purple-600 dark:text-purple-400 hover:text-purple-500 transition-colors"
                        >
                            Sign in
                        </Link>
                    </p>
                </div>
            </div>
        </div>
    );
}

export default function RegisterPage(props: PageProps) {
    const params = use(props.params);
    return (
        <Suspense fallback={<div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading...</div>}>
            <RegisterForm clientId={params.clientId} />
        </Suspense>
    );
}
