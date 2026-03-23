"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";

interface Stats {
    totalUsers: number;
    last24hLogins: number;
}

export default function ClientAdminDashboard() {
    const params = useParams();
    const clientId = params.clientId as string;

    const [stats, setStats] = useState<Stats | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        if (!clientId) return;
        fetch(`/api/client/${clientId}/stats`)
            .then(r => r.json())
            .then(d => { if (d.success) setStats(d.stats); })
            .finally(() => setLoading(false));
    }, [clientId]);

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full min-h-[400px] text-zinc-500">
                Loading stats...
            </div>
        );
    }

    return (
        <div className="p-8 max-w-5xl mx-auto animate-in fade-in slide-in-from-bottom-4 duration-500">
            <header className="mb-8">
                <h1 className="text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-white">Dashboard</h1>
                <p className="text-zinc-500 mt-1">Overview of your application's health and activity.</p>
            </header>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-10">
                <StatCard title="Total Users" value={stats?.totalUsers.toString() ?? "0"} label="Registered accounts" color="indigo" icon={
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
                } />
                <StatCard title="24h Logins" value={stats?.last24hLogins.toString() ?? "0"} label="Sessions in last 24 hours" color="emerald" icon={
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
                } />
                <StatCard title="Uptime" value="100%" label="Service health" color="blue" icon={
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
                } />
            </div>

            {/* Quick links */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <QuickLink href={`/client/${clientId}/users`} title="Manage Users" description="Create, invite, or remove users from your application." color="indigo" />
                <QuickLink href={`/client/${clientId}/settings`} title="App Settings" description="Configure login methods, demo mode, and access controls." color="violet" />
            </div>
        </div>
    );
}

function StatCard({ title, value, label, color, icon }: { title: string; value: string; label: string; color: string; icon: React.ReactNode }) {
    const colors: Record<string, string> = {
        indigo: "text-indigo-600 dark:text-indigo-400 bg-indigo-50 dark:bg-indigo-500/10",
        emerald: "text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-500/10",
        blue: "text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-500/10",
    };
    const textColor = colors[color].split(" ")[0];
    const bgColor = colors[color].split(" ").slice(1).join(" ");

    return (
        <div className="bg-white dark:bg-white/[0.03] p-6 rounded-2xl border border-zinc-200 dark:border-white/10 shadow-sm hover:shadow-md hover:border-indigo-200 dark:hover:border-indigo-500/20 transition-all duration-300">
            <div className={`w-10 h-10 rounded-xl ${bgColor} ${textColor} flex items-center justify-center mb-4`}>
                {icon}
            </div>
            <p className="text-xs font-bold text-zinc-500 uppercase tracking-widest mb-1">{title}</p>
            <p className={`text-4xl font-black ${textColor} mb-1`}>{value}</p>
            <p className="text-sm text-zinc-400">{label}</p>
        </div>
    );
}

function QuickLink({ href, title, description, color }: { href: string; title: string; description: string; color: string }) {
    const colors: Record<string, string> = {
        indigo: "from-indigo-500 to-blue-600",
        violet: "from-violet-500 to-purple-600",
    };
    return (
        <a href={href} className="group bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-2xl p-6 hover:border-indigo-300 dark:hover:border-indigo-500/30 hover:shadow-md transition-all duration-300 block">
            <div className={`w-8 h-1 rounded-full bg-gradient-to-r ${colors[color]} mb-4`} />
            <h3 className="font-bold text-lg text-zinc-900 dark:text-white mb-1 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition-colors">{title}</h3>
            <p className="text-sm text-zinc-500">{description}</p>
        </a>
    );
}
