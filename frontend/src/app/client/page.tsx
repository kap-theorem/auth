"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export default function ClientAdminLogin() {
    const router = useRouter();
    const [clientId, setClientId] = useState("");
    const [clientSecret, setClientSecret] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError("");

        try {
            const res = await fetch("/api/auth/client-login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ clientId, clientSecret }),
            });

            const data = await res.json();

            if (res.ok && data.success) {
                // Navigate to the client's specific dashboard
                router.push(`/client/${clientId}`);
            } else {
                setError(data.message || "Invalid client credentials");
            }
        } catch (err) {
            setError("Failed to authenticate with the server.");
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black p-6">
            <div className="max-w-md w-full bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-white/10 rounded-2xl p-8 shadow-xl">
                <div className="text-center mb-6">
                    <div className="w-12 h-12 bg-indigo-100 dark:bg-indigo-500/20 text-indigo-600 dark:text-indigo-400 rounded-xl flex items-center justify-center mx-auto mb-4">
                        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 002-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" /></svg>
                    </div>
                    <h2 className="text-2xl font-bold text-zinc-900 dark:text-white mb-2">Application Portal</h2>
                    <p className="text-zinc-500 dark:text-zinc-400 text-sm">
                        Enter your Kaplabs Client ID and Secret to manage your app.
                    </p>
                </div>

                {error && (
                    <div className="mb-6 p-3 rounded-lg bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 text-red-600 dark:text-red-400 text-sm font-medium text-center">
                        {error}
                    </div>
                )}

                <form onSubmit={handleLogin} className="space-y-4">
                    <div>
                        <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                            Client ID
                        </label>
                        <input
                            type="text"
                            required
                            value={clientId}
                            onChange={(e) => setClientId(e.target.value)}
                            className="w-full px-4 py-3 rounded-xl bg-zinc-50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-indigo-500 focus:border-transparent outline-none transition-all dark:text-white font-mono text-sm"
                            placeholder="e.g. 555798ba-c23b-4757-..."
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                            Client Secret
                        </label>
                        <input
                            type="password"
                            required
                            value={clientSecret}
                            onChange={(e) => setClientSecret(e.target.value)}
                            className="w-full px-4 py-3 rounded-xl bg-zinc-50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-indigo-500 focus:border-transparent outline-none transition-all dark:text-white font-mono text-sm tracking-widest"
                            placeholder="•••••••••••••••••"
                        />
                    </div>

                    <button
                        type="submit"
                        disabled={loading}
                        className="mt-6 w-full py-3 px-4 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-white font-semibold shadow-lg hover:shadow-xl hover:-translate-y-0.5 transition-all duration-200 disabled:opacity-70 disabled:hover:translate-y-0 disabled:shadow-none flex justify-center items-center"
                    >
                        {loading ? (
                            <svg className="animate-spin h-5 w-5 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                        ) : (
                            "Access Developer Dashboard"
                        )}
                    </button>
                </form>
            </div>
        </div>
    );
}
