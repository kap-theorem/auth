"use client";

import { useState } from "react";
import Link from "next/link";

export default function ForgotPasswordPage() {
    const [email, setEmail] = useState("");
    const [clientId, setClientId] = useState("");
    const [loading, setLoading] = useState(false);
    const [result, setResult] = useState<{ success: boolean; message: string; resetToken?: string } | null>(null);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setResult(null);

        try {
            const res = await fetch("/api/auth/forgot-password", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email, clientId }),
            });
            const data = await res.json();
            setResult(data);
        } catch {
            setResult({ success: false, message: "Failed to connect to server" });
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="relative min-h-screen flex items-center justify-center p-4 overflow-hidden">
            <div className="absolute inset-0 z-0">
                <div className="absolute top-0 right-1/4 w-72 h-72 bg-amber-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob" />
                <div className="absolute bottom-0 left-1/4 w-72 h-72 bg-rose-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob animation-delay-2000" />
            </div>

            <div className="z-10 w-full max-w-md">
                <div className="glass-card rounded-2xl p-8 shadow-2xl relative overflow-hidden">
                    <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-amber-500 via-orange-500 to-rose-500 opacity-80"></div>

                    <div className="text-center mb-8">
                        <h1 className="text-3xl font-bold tracking-tight text-zinc-900 dark:text-white mb-2">
                            Reset Password
                        </h1>
                        <p className="text-sm text-zinc-500 dark:text-zinc-400">
                            Enter your email and client ID to receive a reset token.
                        </p>
                    </div>

                    {result && (
                        <div className={`mb-6 p-4 rounded-xl text-sm font-medium ${result.success
                            ? "bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-200 dark:border-emerald-500/20 text-emerald-700 dark:text-emerald-400"
                            : "bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 text-red-600 dark:text-red-400"
                            }`}>
                            <p>{result.message}</p>
                            {result.resetToken && (
                                <div className="mt-4 p-3 bg-white dark:bg-black/30 border border-zinc-200 dark:border-zinc-700 rounded-lg">
                                    <p className="text-[10px] font-bold text-zinc-500 uppercase tracking-widest mb-1">Reset Token</p>
                                    <p className="text-xs font-mono text-zinc-900 dark:text-white break-all">{result.resetToken}</p>
                                    <Link
                                        href={`/reset-password?token=${result.resetToken}`}
                                        className="mt-3 inline-block text-sm font-semibold text-purple-600 dark:text-purple-400 hover:text-purple-500"
                                    >
                                        Click here to reset your password →
                                    </Link>
                                </div>
                            )}
                        </div>
                    )}

                    <form onSubmit={handleSubmit} className="space-y-5">
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
                                Client ID
                            </label>
                            <input
                                type="text"
                                required
                                value={clientId}
                                onChange={(e) => setClientId(e.target.value)}
                                className="w-full px-4 py-3 rounded-xl bg-white/50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-purple-500 focus:border-transparent outline-none transition-all dark:text-white font-mono text-sm"
                                placeholder="your-client-id"
                            />
                        </div>

                        <button
                            type="submit"
                            disabled={loading}
                            className="w-full py-3 px-4 rounded-xl bg-zinc-900 dark:bg-white text-white dark:text-black font-semibold shadow-lg hover:shadow-xl hover:-translate-y-0.5 transition-all duration-200 disabled:opacity-70 disabled:hover:translate-y-0 flex justify-center items-center"
                        >
                            {loading ? (
                                <svg className="animate-spin h-5 w-5 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                </svg>
                            ) : (
                                "Request Reset Token"
                            )}
                        </button>
                    </form>

                    <p className="mt-8 text-center text-sm text-zinc-500 dark:text-zinc-400">
                        Remember your password?{" "}
                        <Link href="/" className="font-semibold text-purple-600 dark:text-purple-400 hover:text-purple-500 transition-colors">
                            Back to home
                        </Link>
                    </p>
                </div>
            </div>
        </div>
    );
}
