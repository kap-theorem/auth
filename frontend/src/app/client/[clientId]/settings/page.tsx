"use client";

import React, { useEffect, useState } from "react";
import { useParams } from "next/navigation";

interface ClientConfig {
    demo_mode: boolean;
    invite_only: boolean;
    login_type: "both" | "email" | "username";
}

function Toggle({ checked, onChange, label, description }: {
    checked: boolean;
    onChange: (val: boolean) => void;
    label: string;
    description: string;
}) {
    return (
        <div className="flex items-center justify-between py-5 border-b border-zinc-100 dark:border-white/5 last:border-0">
            <div className="flex-1 pr-8">
                <p className="font-bold text-zinc-900 dark:text-white text-sm">{label}</p>
                <p className="text-xs text-zinc-500 mt-0.5">{description}</p>
            </div>
            <button
                type="button"
                role="switch"
                aria-checked={checked}
                onClick={() => onChange(!checked)}
                className={`relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 ${checked ? "bg-indigo-600" : "bg-zinc-200 dark:bg-zinc-700"}`}
            >
                <span
                    aria-hidden="true"
                    className={`pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow-sm transition-transform duration-200 ${checked ? "translate-x-5" : "translate-x-0"}`}
                />
            </button>
        </div>
    );
}

export default function AppSettingsPage() {
    const params = useParams();
    const clientId = params.clientId as string;

    const [config, setConfig] = useState<ClientConfig>({ demo_mode: false, invite_only: false, login_type: "both" });
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [saved, setSaved] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!clientId) return;
        fetch(`/api/client/${clientId}/config`)
            .then(r => r.json())
            .then(d => { if (d.success && d.config) setConfig(d.config); })
            .finally(() => setLoading(false));
    }, [clientId]);

    const saveConfig = async (newConfig: ClientConfig) => {
        setSaving(true);
        setSaved(false);
        setError('');
        try {
            const res = await fetch(`/api/client/${clientId}/config`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    demoMode: newConfig.demo_mode,
                    inviteOnly: newConfig.invite_only,
                    loginType: newConfig.login_type,
                }),
            });
            const data = await res.json();
            if (data.success) {
                setSaved(true);
                setTimeout(() => setSaved(false), 3000);
            } else {
                setError(data.message || "Failed to save settings");
            }
        } catch {
            setError("An error occurred while saving");
        } finally {
            setSaving(false);
        }
    };

    const update = (patch: Partial<ClientConfig>) => {
        const newConfig = { ...config, ...patch };
        setConfig(newConfig);
        saveConfig(newConfig);
    };

    if (loading) {
        return <div className="flex items-center justify-center h-full p-16 text-zinc-400">Loading settings...</div>;
    }

    return (
        <div className="p-8 max-w-2xl mx-auto animate-in fade-in slide-in-from-bottom-4 duration-500">
            <header className="mb-8">
                <h1 className="text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-white">App Settings</h1>
                <p className="text-zinc-500 mt-1">Configure your application's login behavior and access policies.</p>
            </header>

            {/* Status bar */}
            {(saving || saved || error) && (
                <div className={`mb-6 px-4 py-3 rounded-xl text-sm font-medium flex items-center gap-2 ${error ? 'bg-red-50 dark:bg-red-500/10 text-red-600 dark:text-red-400 border border-red-200 dark:border-red-500/20'
                    : saved ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-500/20'
                        : 'bg-blue-50 dark:bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-500/20'
                    }`}>
                    {error ? (
                        <><svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>{error}</>
                    ) : saved ? (
                        <><svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" /></svg>Settings saved</>
                    ) : (
                        <><div className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />Saving…</>
                    )}
                </div>
            )}

            {/* Access Control */}
            <section className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-2xl p-6 mb-6 shadow-sm">
                <h2 className="font-bold text-base text-zinc-900 dark:text-white mb-1">Access Control</h2>
                <p className="text-xs text-zinc-500 mb-4">Restrict who can sign up and how they can log in.</p>

                <Toggle
                    checked={config.invite_only}
                    onChange={val => update({ invite_only: val })}
                    label="Invite Only (Disable Public Signup)"
                    description="Hides the 'Create Account' link on your application's login page. Only admins can provision accounts."
                />

                <Toggle
                    checked={config.demo_mode}
                    onChange={val => update({ demo_mode: val })}
                    label="Demo Mode Visuals"
                    description="Displays demo credentials on the login page so visitors can try the application without a real account."
                />
            </section>

            {/* Login Method */}
            <section className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-2xl p-6 shadow-sm">
                <h2 className="font-bold text-base text-zinc-900 dark:text-white mb-1">Login Identifier</h2>
                <p className="text-xs text-zinc-500 mb-5">Choose which credentials users can submit when logging in.</p>

                <div className="grid grid-cols-3 gap-3">
                    {(['both', 'email', 'username'] as const).map(option => {
                        const labels: Record<string, string> = {
                            both: "Username or Email",
                            email: "Email Only",
                            username: "Username Only",
                        };
                        const icons: Record<string, React.ReactNode> = {
                            both: <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" /></svg>,
                            email: <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>,
                            username: <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>,
                        };
                        const active = config.login_type === option;
                        return (
                            <button
                                key={option}
                                type="button"
                                onClick={() => update({ login_type: option })}
                                className={`flex flex-col items-center gap-2 p-4 rounded-xl border-2 text-center transition-all text-sm font-semibold ${active
                                    ? "border-indigo-500 bg-indigo-50 dark:bg-indigo-500/10 text-indigo-600 dark:text-indigo-400"
                                    : "border-zinc-200 dark:border-white/10 text-zinc-500 hover:border-zinc-300 dark:hover:border-white/20"
                                    }`}
                            >
                                {icons[option]}
                                <span className="text-xs leading-tight">{labels[option]}</span>
                            </button>
                        );
                    })}
                </div>
            </section>
        </div>
    );
}
