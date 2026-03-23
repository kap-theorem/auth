"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

interface LoginFormProps {
    title: string;
    description: string;
    clientId: string;
    redirectUrl?: string; // Optional: If missing, redirects to /dashboard
    showSignUp?: boolean; // Whether to show the "Don't have an account? Sign up" link
    signUpUrl?: string;   // The explicit URL to send users for signup
    hideForgot?: boolean; // Hide "Forgot password?" link
    showDemo?: boolean;   // Show "Login as demo user" link
    loginType?: "both" | "email" | "username"; // The authentication variant
}

export default function LoginForm({
    title,
    description,
    clientId,
    redirectUrl = "/dashboard",
    showSignUp = false,
    signUpUrl = "/register",
    hideForgot = false,
    showDemo = false,
    loginType = "email"
}: LoginFormProps) {
    const router = useRouter();

    const [loginIdentifier, setLoginIdentifier] = useState("");
    const [password, setPassword] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    const performLogin = async (loginEmail: string, loginPassword: string) => {
        setLoading(true);
        setError("");

        try {
            const res = await fetch("/api/auth/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ loginIdentifier: loginEmail, password: loginPassword, clientId }),
            });

            const data = await res.json();

            if (data.success) {
                // Check if we are running in an iframe
                const isIframe = window.self !== window.top;

                if (isIframe) {
                    // Notify parent window of success
                    window.parent.postMessage({
                        type: 'AUTH_SUCCESS',
                        access_token: data.access_token,
                    }, document.referrer || window.location.origin);
                } else {
                    // Normal redirect flow
                    if (redirectUrl.startsWith("http")) {
                        window.location.href = redirectUrl;
                    } else {
                        router.push(redirectUrl);
                    }
                }
            } else {
                setError(data.message || "Invalid credentials");
            }
        } catch (err) {
            setError("Failed to connect to authentication server.");
        } finally {
            setLoading(false);
        }
    };

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        await performLogin(loginIdentifier, password);
    };

    const handleDemoLogin = async (e: React.MouseEvent) => {
        e.preventDefault();
        // Since the demo user is 'demo', use 'demo' as the identifier for demo login type 'both' or 'username'
        // Fallback to email for type 'email' only
        const demoIdent = loginType === "email" ? "demo@kaplabs.dev" : "demo";
        await performLogin(demoIdent, "demo");
    };

    let labelText = "Email Address";
    let inputType = "email";
    let inputPlaceholder = "you@example.com";

    if (loginType === "username") {
        labelText = "Username";
        inputType = "text";
        inputPlaceholder = "username";
    } else if (loginType === "both") {
        labelText = "Email or Username";
        inputType = "text";
        inputPlaceholder = "you@example.com or username";
    }

    return (
        <div className="relative min-h-screen flex items-center justify-center p-4 overflow-hidden">
            {/* Dynamic Background Elements */}
            <div className="absolute inset-0 z-0">
                <div className="absolute top-0 right-1/4 w-72 h-72 bg-blue-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob" />
                <div className="absolute bottom-0 left-1/4 w-72 h-72 bg-purple-500/20 rounded-full mix-blend-multiply filter blur-3xl opacity-70 animate-blob animation-delay-2000" />
            </div>

            <div className="z-10 w-full max-w-md">
                <div className="glass-card rounded-2xl p-8 shadow-2xl relative overflow-hidden">
                    {/* Subtle glow effect top border */}
                    <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-blue-500 via-purple-500 to-pink-500 opacity-80"></div>

                    <div className="text-center mb-8">
                        <h1 className="text-3xl font-bold tracking-tight text-zinc-900 dark:text-white mb-2">
                            {title}
                        </h1>
                        <p className="text-sm text-zinc-500 dark:text-zinc-400">
                            {description}
                        </p>
                    </div>

                    {error && (
                        <div className="mb-6 p-3 rounded-lg bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 text-red-600 dark:text-red-400 text-sm font-medium text-center">
                            {error}
                        </div>
                    )}

                    <form onSubmit={handleLogin} className="space-y-5">
                        <div>
                            <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1">
                                {labelText}
                            </label>
                            <input
                                type={inputType}
                                required
                                value={loginIdentifier}
                                onChange={(e) => setLoginIdentifier(e.target.value)}
                                className="w-full px-4 py-3 rounded-xl bg-white/50 dark:bg-black/50 border border-zinc-200 dark:border-zinc-800 focus:ring-2 focus:ring-purple-500 focus:border-transparent outline-none transition-all dark:text-white"
                                placeholder={inputPlaceholder}
                            />
                        </div>
                        <div>
                            <div className="flex justify-between items-center mb-1">
                                <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300">
                                    Password
                                </label>
                                {!hideForgot && (
                                    <Link
                                        href="/forgot-password"
                                        className="text-xs font-semibold text-purple-600 dark:text-purple-400 hover:text-purple-500 transition-colors"
                                    >
                                        Forgot password?
                                    </Link>
                                )}
                            </div>
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
                            className="w-full py-3 px-4 rounded-xl bg-zinc-900 dark:bg-white text-white dark:text-black font-semibold shadow-lg hover:shadow-xl hover:-translate-y-0.5 transition-all duration-200 disabled:opacity-70 disabled:hover:translate-y-0 flex justify-center items-center"
                        >
                            {loading ? (
                                <svg className="animate-spin h-5 w-5 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                </svg>
                            ) : (
                                "Sign In"
                            )}
                        </button>
                    </form>

                    {showSignUp && (
                        <p className="mt-8 text-center text-sm text-zinc-500 dark:text-zinc-400">
                            Don't have an account?{" "}
                            <Link
                                href={signUpUrl}
                                className="font-semibold text-purple-600 dark:text-purple-400 hover:text-purple-500 transition-colors"
                            >
                                Sign up
                            </Link>
                        </p>
                    )}

                    {showDemo && (
                        <div className="mt-6 text-center border-t border-zinc-200 dark:border-zinc-800 pt-6">
                            <p className="text-sm text-zinc-500 dark:text-zinc-400 mb-3">
                                Want to checkout the site?
                            </p>
                            <button
                                type="button"
                                onClick={handleDemoLogin}
                                className="inline-flex items-center justify-center px-4 py-2 border border-purple-500/30 rounded-xl bg-purple-500/10 text-purple-600 dark:text-purple-400 text-sm font-semibold hover:bg-purple-500/20 transition-colors w-full"
                            >
                                Login as Demo User
                            </button>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
