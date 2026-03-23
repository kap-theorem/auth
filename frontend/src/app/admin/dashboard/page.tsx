"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

interface Stats {
    totalUsers: number;
    totalClients: number;
    activeSessions: number;
    systemHealth: number;
}

interface Client {
    client_id: string;
    client_name: string;
    created_at: { seconds: number; nanos: number };
}

export default function SuperAdminDashboard() {
    const [stats, setStats] = useState<Stats | null>(null);
    const [clients, setClients] = useState<Client[]>([]);
    const [loading, setLoading] = useState(true);

    const [isRegisterModalOpen, setIsRegisterModalOpen] = useState(false);
    const [newClientName, setNewClientName] = useState("");
    const [registrationResult, setRegistrationResult] = useState<{ clientId: string, clientSecret: string } | null>(null);
    const [isRegistering, setIsRegistering] = useState(false);

    const handleRegisterClient = async (e: React.FormEvent) => {
        e.preventDefault();
        setIsRegistering(true);
        try {
            const res = await fetch('/api/admin/clients/register', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ clientName: newClientName }),
            });
            const data = await res.json();
            if (data.success) {
                setRegistrationResult({ clientId: data.clientId, clientSecret: data.clientSecret });
                // Re-fetch clients to update the list
                const clientsRes = await fetch('/api/admin/clients');
                const clientsData = await clientsRes.json();
                if (clientsData.success) setClients(clientsData.clients);
            } else {
                alert(data.message || "Failed to register application");
            }
        } catch (error) {
            console.error("Registration error:", error);
            alert("An error occurred during registration");
        } finally {
            setIsRegistering(false);
        }
    };

    useEffect(() => {
        const fetchData = async () => {
            try {
                const [statsRes, clientsRes] = await Promise.all([
                    fetch('/api/admin/stats'),
                    fetch('/api/admin/clients')
                ]);

                const statsData = await statsRes.json();
                const clientsData = await clientsRes.json();

                if (statsData.success) setStats(statsData.stats);
                if (clientsData.success) setClients(clientsData.clients);
            } catch (error) {
                console.error("Failed to fetch dashboard data:", error);
            } finally {
                setLoading(false);
            }
        };

        fetchData();
        const interval = setInterval(fetchData, 10000); // Refresh every 10s
        return () => clearInterval(interval);
    }, []);

    if (loading && !stats) {
        return <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Initializing Command Center...</div>;
    }

    return (
        <div className="max-w-6xl mx-auto w-full animate-in fade-in slide-in-from-bottom-4 duration-500 py-10 px-4">
            <header className="mb-12 flex justify-between items-end">
                <div>
                    <h1 className="text-4xl font-extrabold tracking-tight mb-2 text-black dark:text-white">Command Center</h1>
                    <p className="text-lg text-zinc-500 dark:text-zinc-400">Global system oversight and client management.</p>
                </div>
                <div className="flex gap-4">
                    <button
                        onClick={() => setIsRegisterModalOpen(true)}
                        className="px-4 py-2 bg-zinc-900 dark:bg-white text-white dark:text-black rounded-xl font-bold hover:shadow-lg transition-all"
                    >
                        + New App
                    </button>
                    <button
                        onClick={() => {
                            document.cookie = "admin_access_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
                            window.location.href = "/admin";
                        }}
                        className="px-4 py-2 border border-zinc-200 dark:border-white/10 rounded-xl font-bold hover:bg-zinc-100 dark:hover:bg-white/5 transition-all text-black dark:text-white"
                    >
                        Sign Out
                    </button>
                </div>
            </header>

            {/* Registration Modal */}
            {isRegisterModalOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-300">
                    <div className="bg-white dark:bg-zinc-900 w-full max-w-md rounded-3xl p-8 shadow-2xl border border-zinc-200 dark:border-white/10 animate-in zoom-in-95 duration-300">
                        {!registrationResult ? (
                            <>
                                <h3 className="text-2xl font-black text-black dark:text-white mb-2">Register Application</h3>
                                <p className="text-zinc-500 dark:text-zinc-400 mb-6 text-sm">Create a new entry in the Kaplabs Ecosystem.</p>
                                <form onSubmit={handleRegisterClient} className="space-y-4">
                                    <div>
                                        <label className="block text-xs font-bold text-zinc-400 uppercase tracking-widest mb-1">Application Name</label>
                                        <input
                                            type="text"
                                            required
                                            value={newClientName}
                                            onChange={(e) => setNewClientName(e.target.value)}
                                            placeholder="e.g. Wordskali"
                                            className="w-full px-4 py-3 bg-zinc-50 dark:bg-black border border-zinc-200 dark:border-zinc-800 rounded-xl outline-none focus:ring-2 focus:ring-indigo-500 transition-all text-black dark:text-white"
                                        />
                                    </div>
                                    <div className="flex gap-3 pt-2">
                                        <button
                                            type="button"
                                            onClick={() => setIsRegisterModalOpen(false)}
                                            className="flex-1 py-3 border border-zinc-200 dark:border-white/10 rounded-xl font-bold text-zinc-500 hover:bg-zinc-50 dark:hover:bg-white/5 transition-all"
                                        >
                                            Cancel
                                        </button>
                                        <button
                                            type="submit"
                                            disabled={isRegistering}
                                            className="flex-1 py-3 bg-indigo-600 text-white rounded-xl font-bold hover:bg-indigo-700 transition-all disabled:opacity-50"
                                        >
                                            {isRegistering ? "Registering..." : "Provision App"}
                                        </button>
                                    </div>
                                </form>
                            </>
                        ) : (
                            <div className="animate-in fade-in duration-500">
                                <div className="w-16 h-16 bg-emerald-500/10 rounded-full flex items-center justify-center mb-6 mx-auto">
                                    <svg className="w-8 h-8 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" /></svg>
                                </div>
                                <h3 className="text-2xl font-black text-black dark:text-white text-center mb-2">Registration Successful</h3>
                                <p className="text-zinc-500 dark:text-zinc-400 text-center text-sm mb-8">Copy these credentials now. They will not be shown again.</p>

                                <div className="space-y-4 mb-8">
                                    <CredentialField label="Client ID" value={registrationResult.clientId} />
                                    <CredentialField label="Client Secret" value={registrationResult.clientSecret} sensitive />
                                </div>

                                <button
                                    onClick={() => {
                                        setIsRegisterModalOpen(false);
                                        setRegistrationResult(null);
                                        setNewClientName("");
                                    }}
                                    className="w-full py-4 bg-zinc-900 dark:bg-white text-white dark:text-black rounded-2xl font-black uppercase tracking-widest hover:shadow-xl transition-all"
                                >
                                    Dismiss Credentials
                                </button>
                            </div>
                        )}
                    </div>
                </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-4 gap-6 mb-12">
                <StatCard
                    title="Total Population"
                    value={stats?.totalUsers.toLocaleString() || "0"}
                    label="Registered Identities"
                    color="indigo"
                />
                <StatCard
                    title="Ecosystem Size"
                    value={stats?.totalClients.toString() || "0"}
                    label="Active Applications"
                    color="blue"
                />
                <StatCard
                    title="Traffic"
                    value={stats?.activeSessions.toString() || "0"}
                    label="Live Sessions"
                    color="emerald"
                />
                <StatCard
                    title="System pulse"
                    value={`${((stats?.systemHealth || 1) * 100).toFixed(0)}%`}
                    label="Operational Health"
                    color="amber"
                />
            </div>

            <section className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-3xl overflow-hidden shadow-2xl backdrop-blur-3xl">
                <div className="px-8 py-6 border-b border-zinc-200 dark:border-white/10 bg-zinc-50/50 dark:bg-transparent flex justify-between items-center">
                    <h3 className="font-bold text-xl text-black dark:text-white">Registered Applications</h3>
                    <span className="text-sm font-medium text-zinc-400">{clients.length} Total</span>
                </div>
                <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    {clients.map((client) => (
                        <ClientCard key={client.client_id} client={client} />
                    ))}
                </div>
            </section>
        </div>
    );
}

function CredentialField({ label, value, sensitive }: { label: string, value: string, sensitive?: boolean }) {
    const [hidden, setHidden] = useState(sensitive || false);

    return (
        <div className="p-4 bg-zinc-50 dark:bg-black border border-zinc-100 dark:border-zinc-800 rounded-2xl relative group">
            <p className="text-[10px] font-bold text-zinc-400 uppercase tracking-widest mb-1">{label}</p>
            <div className="flex items-center justify-between gap-4">
                <p className={`text-sm font-mono text-zinc-900 dark:text-white break-all ${hidden ? 'blur-sm select-none' : ''}`}>
                    {value}
                </p>
                <div className="flex gap-2">
                    {sensitive && (
                        <button
                            onClick={() => setHidden(!hidden)}
                            className="p-1.5 text-zinc-400 hover:text-black dark:hover:text-white transition-colors"
                        >
                            {hidden ? (
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
                            ) : (
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.888 9.888L3 3m18 18l-6.888-6.888" /></svg>
                            )}
                        </button>
                    )}
                    <button
                        onClick={() => navigator.clipboard.writeText(value)}
                        className="p-1.5 text-zinc-400 hover:text-indigo-500 transition-colors"
                    >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                    </button>
                </div>
            </div>
        </div>
    );
}

function StatCard({ title, value, label, color }: { title: string, value: string, label: string, color: string }) {
    const colors: Record<string, string> = {
        indigo: "text-indigo-600 dark:text-indigo-400",
        blue: "text-blue-600 dark:text-blue-400",
        emerald: "text-emerald-600 dark:text-emerald-400",
        amber: "text-amber-600 dark:text-amber-400",
    };

    return (
        <div className="bg-white dark:bg-white/[0.03] p-6 rounded-3xl border border-zinc-200 dark:border-white/10 shadow-lg hover:-translate-y-1 transition-all duration-300">
            <p className="text-xs font-bold text-zinc-500 dark:text-zinc-500 uppercase tracking-widest mb-4">{title}</p>
            <p className={`text-4xl font-black mb-1 ${colors[color]}`}>{value}</p>
            <p className="text-sm font-medium text-zinc-400">{label}</p>
        </div>
    );
}

function ClientCard({ client }: { client: Client }) {
    return (
        <div className="p-6 rounded-2xl border border-zinc-100 dark:border-white/5 bg-zinc-50/50 dark:bg-white/5 hover:border-indigo-500/50 transition-all group flex flex-col justify-between h-40">
            <div>
                <div className="flex justify-between items-start mb-2">
                    <div className="w-10 h-10 rounded-xl bg-white dark:bg-black border border-zinc-200 dark:border-white/10 flex items-center justify-center mb-3">
                        <svg className="w-6 h-6 text-zinc-400 group-hover:text-indigo-500 transition-colors" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
                    </div>
                    <span className="text-[10px] font-bold py-1 px-2 bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 rounded-full uppercase tracking-tighter">Active</span>
                </div>
                <h4 className="font-bold text-lg text-black dark:text-white truncate">{client.client_name}</h4>
                <p className="text-xs font-mono text-zinc-400 truncate">{client.client_id}</p>
            </div>
            <Link
                href={`/client/${client.client_id}`}
                className="text-sm font-bold text-indigo-500 hover:underline flex items-center gap-1 opacity-100 md:opacity-0 group-hover:opacity-100 transition-opacity"
            >
                Enter Portal
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
            </Link>
        </div>
    );
}
