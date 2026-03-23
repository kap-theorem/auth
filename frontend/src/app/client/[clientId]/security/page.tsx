"use client";

import { useState } from "react";
import { useParams } from "next/navigation";

export default function SecurityPage() {
    const params = useParams();
    const clientId = params.clientId as string;

    const [copied, setCopied] = useState(false);
    const [showRotate, setShowRotate] = useState(false);
    const [currentSecret, setCurrentSecret] = useState("");
    const [rotateLoading, setRotateLoading] = useState(false);
    const [rotateResult, setRotateResult] = useState<{ success: boolean; message: string; newSecret?: string } | null>(null);

    const copyClientId = () => {
        navigator.clipboard.writeText(clientId);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    const handleRotateSecret = async (e: React.FormEvent) => {
        e.preventDefault();
        setRotateLoading(true);
        setRotateResult(null);

        try {
            const res = await fetch(`/api/client/${clientId}/security/rotate-secret`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ currentSecret }),
            });
            const data = await res.json();
            if (data.success) {
                setRotateResult({
                    success: true,
                    message: "Secret rotated successfully. Copy your new secret now — it won't be shown again.",
                    newSecret: data.client_secret,
                });
                setCurrentSecret("");
            } else {
                setRotateResult({ success: false, message: data.message || "Failed to rotate secret" });
            }
        } catch {
            setRotateResult({ success: false, message: "An error occurred" });
        } finally {
            setRotateLoading(false);
        }
    };

    return (
        <div className="p-8 max-w-2xl mx-auto animate-in fade-in slide-in-from-bottom-4 duration-500">
            <header className="mb-8">
                <h1 className="text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-white">Security</h1>
                <p className="text-zinc-500 mt-1">Manage your application credentials and access keys.</p>
            </header>

            {/* Client ID */}
            <section className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/[0.06] rounded-2xl p-6 mb-6">
                <h2 className="font-bold text-sm text-zinc-900 dark:text-white mb-1">Client ID</h2>
                <p className="text-xs text-zinc-500 mb-4">Your application&apos;s unique identifier. Use this in API calls and SDK configuration.</p>

                <div className="flex items-center gap-3 p-4 bg-zinc-50 dark:bg-black/40 border border-zinc-200 dark:border-white/[0.06] rounded-xl">
                    <code className="flex-1 text-sm font-mono text-zinc-700 dark:text-zinc-300 break-all">{clientId}</code>
                    <button
                        onClick={copyClientId}
                        className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
                            copied
                                ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                                : 'bg-zinc-100 dark:bg-white/5 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-white/10'
                        }`}
                    >
                        {copied ? "Copied!" : "Copy"}
                    </button>
                </div>
            </section>

            {/* Client Secret Rotation */}
            <section className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/[0.06] rounded-2xl p-6 mb-6">
                <h2 className="font-bold text-sm text-zinc-900 dark:text-white mb-1">Client Secret</h2>
                <p className="text-xs text-zinc-500 mb-4">
                    Your secret is stored securely and cannot be viewed. If compromised, rotate it below.
                </p>

                <div className="flex items-center gap-3 p-4 bg-zinc-50 dark:bg-black/40 border border-zinc-200 dark:border-white/[0.06] rounded-xl mb-4">
                    <code className="flex-1 text-sm font-mono text-zinc-400">{"*".repeat(40)}</code>
                </div>

                {!showRotate ? (
                    <button
                        onClick={() => setShowRotate(true)}
                        className="px-4 py-2.5 rounded-xl text-sm font-bold bg-zinc-100 dark:bg-white/5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-white/10 transition-colors"
                    >
                        Rotate Secret
                    </button>
                ) : (
                    <div className="mt-2 p-4 bg-amber-50 dark:bg-amber-500/5 border border-amber-200 dark:border-amber-500/20 rounded-xl">
                        <p className="text-xs font-semibold text-amber-700 dark:text-amber-400 mb-3">
                            Enter your current secret to generate a new one. All existing integrations using the old secret will stop working.
                        </p>
                        <form onSubmit={handleRotateSecret} className="space-y-3">
                            <input
                                type="password"
                                required
                                placeholder="Current client secret"
                                value={currentSecret}
                                onChange={(e) => setCurrentSecret(e.target.value)}
                                className="w-full px-4 py-3 bg-white dark:bg-black border border-zinc-200 dark:border-white/10 rounded-xl text-sm text-zinc-900 dark:text-white focus:outline-none focus:border-amber-500 transition-colors"
                            />
                            <div className="flex gap-2">
                                <button
                                    type="button"
                                    onClick={() => { setShowRotate(false); setRotateResult(null); }}
                                    className="px-4 py-2.5 rounded-xl text-sm font-bold bg-zinc-100 dark:bg-white/5 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-white/10 transition-colors"
                                >
                                    Cancel
                                </button>
                                <button
                                    type="submit"
                                    disabled={rotateLoading}
                                    className="px-4 py-2.5 rounded-xl text-sm font-bold bg-amber-600 text-white hover:bg-amber-500 transition-colors disabled:opacity-60"
                                >
                                    {rotateLoading ? "Rotating..." : "Confirm Rotation"}
                                </button>
                            </div>
                        </form>

                        {rotateResult && (
                            <div className={`mt-3 p-3 rounded-xl text-sm ${
                                rotateResult.success
                                    ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-500/20'
                                    : 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-400 border border-red-200 dark:border-red-500/20'
                            }`}>
                                <p className="font-medium mb-1">{rotateResult.message}</p>
                                {rotateResult.newSecret && (
                                    <div className="mt-2 p-3 bg-white dark:bg-black rounded-lg border border-zinc-200 dark:border-white/10">
                                        <p className="text-[10px] font-bold text-zinc-500 uppercase tracking-widest mb-1">New Secret</p>
                                        <code className="text-xs font-mono text-zinc-900 dark:text-white break-all">{rotateResult.newSecret}</code>
                                        <button
                                            onClick={() => {
                                                navigator.clipboard.writeText(rotateResult.newSecret!);
                                            }}
                                            className="mt-2 w-full py-2 rounded-lg text-xs font-bold bg-zinc-900 dark:bg-white text-white dark:text-black hover:opacity-90 transition-opacity"
                                        >
                                            Copy New Secret
                                        </button>
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                )}
            </section>

            {/* Danger Zone */}
            <section className="bg-white dark:bg-white/[0.03] border border-red-200 dark:border-red-500/20 rounded-2xl p-6">
                <h2 className="font-bold text-sm text-red-600 dark:text-red-400 mb-1">Danger Zone</h2>
                <p className="text-xs text-zinc-500 mb-4">Irreversible actions that affect all users of this application.</p>

                <div className="flex items-center justify-between py-4 border-t border-zinc-100 dark:border-white/5">
                    <div>
                        <p className="text-sm font-semibold text-zinc-900 dark:text-white">Revoke All Sessions</p>
                        <p className="text-xs text-zinc-500">Force logout all users currently signed into this application.</p>
                    </div>
                    <button
                        onClick={async () => {
                            if (!confirm("This will force logout ALL users of this application. Continue?")) return;
                            // This would need a new endpoint — placeholder for now
                            alert("This feature requires an admin-level session revocation endpoint.");
                        }}
                        className="shrink-0 px-4 py-2 rounded-xl text-xs font-bold border border-red-200 dark:border-red-500/30 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
                    >
                        Revoke All
                    </button>
                </div>
            </section>
        </div>
    );
}
