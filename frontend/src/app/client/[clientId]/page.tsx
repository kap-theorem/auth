"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from "recharts";

interface Stats {
    totalUsers: number;
    last24hLogins: number;
    newUsersLast7d: number;
    activeSessions: number;
}

interface User {
    user_id: string;
    username: string;
    email: string;
    created_at: { seconds: number; nanos: number };
}

interface DailyLogin {
    date: string;
    count: number;
}

export default function ClientAdminDashboard() {
    const params = useParams();
    const clientId = params.clientId as string;

    const [stats, setStats] = useState<Stats | null>(null);
    const [activity, setActivity] = useState<DailyLogin[]>([]);
    const [recentUsers, setRecentUsers] = useState<User[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        if (!clientId) return;

        Promise.all([
            fetch(`/api/client/${clientId}/stats`).then(r => r.json()),
            fetch(`/api/client/${clientId}/activity?days=30`).then(r => r.json()),
            fetch(`/api/client/${clientId}/users`).then(r => r.json()),
        ])
            .then(([statsData, activityData, usersData]) => {
                if (statsData.success) setStats(statsData.stats || statsData);
                if (activityData.success) setActivity(activityData.daily_logins || []);
                if (usersData.success) {
                    // Sort by created_at descending and take the latest 5
                    const sorted = (usersData.users || [])
                        .sort((a: User, b: User) => (b.created_at?.seconds || 0) - (a.created_at?.seconds || 0))
                        .slice(0, 5);
                    setRecentUsers(sorted);
                }
            })
            .finally(() => setLoading(false));
    }, [clientId]);

    // Format chart data — fill missing days with 0
    const chartData = (() => {
        const map = new Map(activity.map(d => [d.date, d.count]));
        const days: { date: string; logins: number }[] = [];
        for (let i = 29; i >= 0; i--) {
            const d = new Date();
            d.setDate(d.getDate() - i);
            const key = d.toISOString().split('T')[0];
            days.push({ date: key, logins: map.get(key) || 0 });
        }
        return days;
    })();

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full min-h-[400px]">
                <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin" />
            </div>
        );
    }

    const statCards = [
        { title: "Total Users", value: stats?.totalUsers ?? 0, label: "Registered accounts", color: "indigo" },
        { title: "24h Logins", value: stats?.last24hLogins ?? 0, label: "Sessions today", color: "emerald" },
        { title: "New Users (7d)", value: stats?.newUsersLast7d ?? 0, label: "Recent signups", color: "blue" },
        { title: "Active Sessions", value: stats?.activeSessions ?? 0, label: "Live right now", color: "violet" },
    ];

    return (
        <div className="p-8 max-w-6xl mx-auto animate-in fade-in slide-in-from-bottom-4 duration-500">
            <header className="mb-8">
                <h1 className="text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-white">Dashboard</h1>
                <p className="text-zinc-500 mt-1">Overview of your application&apos;s health and activity.</p>
            </header>

            {/* Stats Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
                {statCards.map((card) => (
                    <StatCard key={card.title} {...card} />
                ))}
            </div>

            {/* Chart + Recent Users */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-8">
                {/* Login Activity Chart */}
                <div className="lg:col-span-2 bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/[0.06] rounded-2xl p-6">
                    <div className="flex items-center justify-between mb-6">
                        <div>
                            <h3 className="font-bold text-zinc-900 dark:text-white">Login Activity</h3>
                            <p className="text-xs text-zinc-500 mt-0.5">Daily logins over the last 30 days</p>
                        </div>
                    </div>
                    <div className="h-64">
                        <ResponsiveContainer width="100%" height="100%">
                            <BarChart data={chartData} margin={{ top: 0, right: 0, left: -20, bottom: 0 }}>
                                <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.06)" vertical={false} />
                                <XAxis
                                    dataKey="date"
                                    tick={{ fontSize: 10, fill: '#71717a' }}
                                    tickFormatter={(v) => {
                                        const d = new Date(v + 'T00:00:00');
                                        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
                                    }}
                                    interval={6}
                                    axisLine={false}
                                    tickLine={false}
                                />
                                <YAxis
                                    tick={{ fontSize: 10, fill: '#71717a' }}
                                    axisLine={false}
                                    tickLine={false}
                                    allowDecimals={false}
                                />
                                <Tooltip
                                    contentStyle={{
                                        background: '#18181b',
                                        border: '1px solid rgba(255,255,255,0.1)',
                                        borderRadius: '12px',
                                        fontSize: '12px',
                                        color: '#fff',
                                    }}
                                    labelFormatter={(v) => {
                                        const d = new Date(v + 'T00:00:00');
                                        return d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
                                    }}
                                    cursor={{ fill: 'rgba(99,102,241,0.08)' }}
                                />
                                <Bar dataKey="logins" fill="#6366f1" radius={[4, 4, 0, 0]} maxBarSize={24} />
                            </BarChart>
                        </ResponsiveContainer>
                    </div>
                </div>

                {/* Recent Users */}
                <div className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/[0.06] rounded-2xl p-6">
                    <div className="flex items-center justify-between mb-4">
                        <h3 className="font-bold text-zinc-900 dark:text-white">Recent Users</h3>
                        <a href={`/client/${clientId}/users`} className="text-xs font-semibold text-indigo-500 hover:text-indigo-400 transition-colors">
                            View all
                        </a>
                    </div>
                    {recentUsers.length === 0 ? (
                        <p className="text-sm text-zinc-400 py-8 text-center">No users yet</p>
                    ) : (
                        <div className="space-y-3">
                            {recentUsers.map((user) => (
                                <div key={user.user_id} className="flex items-center gap-3">
                                    <div className="w-8 h-8 rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 flex items-center justify-center text-white text-xs font-bold shrink-0">
                                        {user.username[0]?.toUpperCase()}
                                    </div>
                                    <div className="min-w-0 flex-1">
                                        <p className="text-sm font-semibold text-zinc-900 dark:text-white truncate">{user.username}</p>
                                        <p className="text-[11px] text-zinc-400 truncate">{user.email}</p>
                                    </div>
                                    <p className="text-[10px] text-zinc-400 shrink-0">
                                        {new Date((user.created_at?.seconds || 0) * 1000).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                                    </p>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            </div>

            {/* Quick Actions */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <QuickAction
                    href={`/client/${clientId}/users`}
                    title="Manage Users"
                    description="Create, invite, or remove users"
                    icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" /></svg>}
                />
                <QuickAction
                    href={`/client/${clientId}/settings`}
                    title="App Settings"
                    description="Configure login and access controls"
                    icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>}
                />
                <QuickAction
                    href={`/client/${clientId}/security`}
                    title="Security"
                    description="Credentials and secret rotation"
                    icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>}
                />
            </div>
        </div>
    );
}

function StatCard({ title, value, label, color }: { title: string; value: number; label: string; color: string }) {
    const colorMap: Record<string, { text: string; bg: string }> = {
        indigo: { text: "text-indigo-600 dark:text-indigo-400", bg: "bg-indigo-50 dark:bg-indigo-500/10" },
        emerald: { text: "text-emerald-600 dark:text-emerald-400", bg: "bg-emerald-50 dark:bg-emerald-500/10" },
        blue: { text: "text-blue-600 dark:text-blue-400", bg: "bg-blue-50 dark:bg-blue-500/10" },
        violet: { text: "text-violet-600 dark:text-violet-400", bg: "bg-violet-50 dark:bg-violet-500/10" },
    };
    const c = colorMap[color] || colorMap.indigo;

    return (
        <div className="bg-white dark:bg-white/[0.03] p-5 rounded-2xl border border-zinc-200 dark:border-white/[0.06] hover:border-zinc-300 dark:hover:border-white/10 transition-all">
            <p className="text-[11px] font-bold text-zinc-500 uppercase tracking-widest mb-3">{title}</p>
            <p className={`text-3xl font-black ${c.text} mb-1`}>{value.toLocaleString()}</p>
            <p className="text-xs text-zinc-400">{label}</p>
        </div>
    );
}

function QuickAction({ href, title, description, icon }: { href: string; title: string; description: string; icon: React.ReactNode }) {
    return (
        <a href={href} className="group bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/[0.06] rounded-2xl p-5 hover:border-indigo-300 dark:hover:border-indigo-500/30 transition-all block">
            <div className="w-10 h-10 rounded-xl bg-zinc-100 dark:bg-white/5 flex items-center justify-center mb-3 text-zinc-400 group-hover:text-indigo-500 transition-colors">
                {icon}
            </div>
            <h3 className="font-bold text-sm text-zinc-900 dark:text-white mb-0.5 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition-colors">{title}</h3>
            <p className="text-xs text-zinc-500">{description}</p>
        </a>
    );
}
