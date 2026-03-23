"use client";

import { useEffect, useState } from "react";

interface UserProfile {
    userId: string;
    username: string;
    email: string;
    clientId: string;
    createdAt: { seconds: number; nanos: number };
}

export default function UserDashboard() {
    const [profile, setProfile] = useState<UserProfile | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        const fetchProfile = async () => {
            try {
                const res = await fetch('/api/user/profile');
                const data = await res.json();
                if (data.success) {
                    setProfile(data.user);
                } else {
                    // Redirect to login if not authenticated
                    window.location.href = "/";
                }
            } catch (error) {
                console.error("Failed to fetch profile:", error);
            } finally {
                setLoading(false);
            }
        };

        fetchProfile();
    }, []);

    if (loading) {
        return <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Securing your session...</div>;
    }

    if (!profile) return null;

    return (
        <div className="max-w-5xl mx-auto w-full animate-in fade-in slide-in-from-bottom-4 duration-500 py-10 px-4">
            <header className="mb-12 flex justify-between items-end">
                <div>
                    <h1 className="text-4xl font-extrabold tracking-tight mb-2 text-black dark:text-white">Profile Portal</h1>
                    <p className="text-lg text-zinc-500 dark:text-zinc-400 font-medium">Manage your personal identity across the Kaplabs ecosystem.</p>
                </div>
                <button
                    onClick={() => {
                        document.cookie = "access_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
                        document.cookie = "refresh_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
                        window.location.href = "/";
                    }}
                    className="px-6 py-2.5 bg-zinc-100 dark:bg-white/5 text-zinc-900 dark:text-white rounded-2xl font-bold border border-zinc-200 dark:border-white/10 hover:bg-zinc-200 dark:hover:bg-white/10 transition-all shadow-sm"
                >
                    Sign Out
                </button>
            </header>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                <main className="lg:col-span-2 space-y-8">
                    {/* Identity Card */}
                    <div className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-3xl p-8 shadow-2xl backdrop-blur-3xl overflow-hidden relative">
                        <div className="absolute top-0 right-0 w-32 h-32 bg-indigo-500/5 blur-3xl rounded-full"></div>
                        <div className="flex items-center gap-6 mb-8">
                            <div className="w-20 h-20 rounded-3xl bg-gradient-to-br from-indigo-500 to-blue-600 flex items-center justify-center text-white text-3xl font-black shadow-inner border border-white/20">
                                {profile.username[0].toUpperCase()}
                            </div>
                            <div>
                                <h3 className="text-2xl font-black text-black dark:text-white">{profile.username}</h3>
                                <p className="text-zinc-500 font-medium">{profile.email}</p>
                            </div>
                        </div>

                        <div className="grid grid-cols-2 gap-4">
                            <InfoField label="User ID" value={profile.userId} mono />
                            <InfoField label="Primary Client" value={profile.clientId} mono />
                            <InfoField label="Identity Created" value={new Date(profile.createdAt.seconds * 1000).toLocaleDateString()} />
                            <InfoField label="Member Status" value="Verified Global Identity" color="emerald" />
                        </div>
                    </div>

                    {/* Security Actions */}
                    <div className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-3xl p-8 shadow-xl">
                        <h4 className="font-black text-xl text-black dark:text-white mb-6 flex items-center gap-2">
                            <svg className="w-6 h-6 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>
                            Security Controls
                        </h4>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <button className="p-4 bg-zinc-50 dark:bg-white/5 border border-zinc-100 dark:border-white/5 rounded-2xl text-left hover:border-indigo-500/50 transition-all group">
                                <p className="font-bold text-black dark:text-white group-hover:text-indigo-500 transition-colors">Rotate Password</p>
                                <p className="text-xs text-zinc-500 mt-1">Recommended every 90 days</p>
                            </button>
                            <button className="p-4 bg-zinc-50 dark:bg-white/5 border border-zinc-100 dark:border-white/5 rounded-2xl text-left hover:border-indigo-500/50 transition-all group">
                                <p className="font-bold text-black dark:text-white group-hover:text-indigo-500 transition-colors">Enable Two-Factor</p>
                                <p className="text-xs text-zinc-500 mt-1">Add an extra layer of protection</p>
                            </button>
                        </div>
                    </div>
                </main>

                <aside className="space-y-6">
                    <div className="bg-zinc-900 dark:bg-white p-8 rounded-3xl shadow-2xl overflow-hidden relative group cursor-pointer">
                        <div className="relative z-10">
                            <h4 className="font-black text-xl text-white dark:text-black mb-4">Kaplabs Cloud</h4>
                            <p className="text-zinc-400 dark:text-zinc-500 text-sm mb-6 leading-relaxed">Your identity is managed by Antigravity Auth, providing seamless access to all connected applications.</p>
                            <div className="flex -space-x-2">
                                <AppIcon name="W" color="bg-orange-500" />
                                <AppIcon name="B" color="bg-emerald-500" />
                                <AppIcon name="L" color="bg-indigo-500" />
                            </div>
                        </div>
                        <div className="absolute top-0 right-0 p-4 opacity-20 group-hover:opacity-100 group-hover:translate-x-1 group-hover:-translate-y-1 transition-all">
                            <svg className="w-8 h-8 text-white dark:text-black" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
                        </div>
                    </div>

                    <div className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 p-8 rounded-3xl shadow-lg backdrop-blur-3xl">
                        <h4 className="font-bold text-lg text-black dark:text-white mb-6 tracking-tight">Active Sessions</h4>
                        <div className="space-y-6">
                            <SessionItem device="Chrome on MacOS" location="San Francisco, US" current />
                            <SessionItem device="Safari on iPhone" location="San Francisco, US" />
                        </div>
                        <button className="w-full mt-8 py-3 text-red-500 font-bold text-sm bg-red-500/5 rounded-2xl hover:bg-red-500/10 transition-colors">
                            Revoke All Other Sessions
                        </button>
                    </div>
                </aside>
            </div>
        </div>
    );
}

function InfoField({ label, value, mono, color }: { label: string, value: string, mono?: boolean, color?: string }) {
    const colors: Record<string, string> = {
        emerald: "text-emerald-500",
    };

    return (
        <div className="p-4 bg-zinc-50/50 dark:bg-white/5 border border-zinc-100 dark:border-white/5 rounded-2xl">
            <p className="text-[10px] font-bold text-zinc-500 uppercase tracking-widest mb-1.5">{label}</p>
            <p className={`text-sm font-bold ${mono ? 'font-mono' : ''} ${color ? colors[color] : 'text-zinc-900 dark:text-white'} truncate`}>{value}</p>
        </div>
    );
}

function SessionItem({ device, location, current }: { device: string, location: string, current?: boolean }) {
    return (
        <div className="flex items-center gap-4 relative">
            <div className={`w-2 h-2 rounded-full ${current ? 'bg-emerald-500 animate-pulse ring-4 ring-emerald-500/20' : 'bg-zinc-300 dark:bg-zinc-700'}`}></div>
            <div>
                <p className="font-bold text-sm text-black dark:text-white flex items-center gap-2">
                    {device}
                    {current && <span className="text-[10px] bg-emerald-500/10 text-emerald-600 px-1.5 py-0.5 rounded-full uppercase tracking-tighter">Current</span>}
                </p>
                <p className="text-xs text-zinc-500">{location}</p>
            </div>
        </div>
    );
}

function AppIcon({ name, color }: { name: string, color: string }) {
    return (
        <div className={`w-8 h-8 rounded-lg ${color} flex items-center justify-center text-white text-[10px] font-black border-2 border-white dark:border-zinc-900 shadow-lg`}>
            {name}
        </div>
    );
}
