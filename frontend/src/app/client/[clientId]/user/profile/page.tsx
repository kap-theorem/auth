"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";

export default function ProfilePage() {
    const router = useRouter();
    const [user, setUser] = useState<any>(null);
    const [loading, setLoading] = useState(true);

    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");

    const [currentPassword, setCurrentPassword] = useState("");
    const [newPassword, setNewPassword] = useState("");

    const [profileMessage, setProfileMessage] = useState({ text: "", type: "" });
    const [passwordMessage, setPasswordMessage] = useState({ text: "", type: "" });

    const [locks, setLocks] = useState({ lockUsername: false, lockEmail: false, lockPassword: false });

    useEffect(() => {
        // Fetch current session details
        const fetchSession = async () => {
            try {
                const res = await fetch("/api/auth/session");
                const data = await res.json();
                if (data.authenticated && data.user) {
                    setUser(data.user);
                    setUsername(data.user.username);
                    setEmail(data.user.email);
                    setLocks({
                        lockUsername: data.user.lockUsername || false,
                        lockEmail: data.user.lockEmail || false,
                        lockPassword: data.user.lockPassword || false,
                    });
                } else {
                    // Not authenticated, redirect to login via iframe message or normal redirect
                    const isIframe = window.self !== window.top;
                    if (isIframe) {
                        window.parent.postMessage({ type: 'AUTH_LOGOUT' }, '*');
                    } else {
                        window.location.href = "/";
                    }
                }
            } catch (err) {
                console.error("Session fetch failed", err);
            } finally {
                setLoading(false);
            }
        };
        fetchSession();
    }, []);

    const handleUpdateProfile = async (e: React.FormEvent) => {
        e.preventDefault();
        setProfileMessage({ text: "Updating...", type: "info" });

        try {
            const res = await fetch("/api/auth/profile", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ newUsername: username, newEmail: email })
            });
            const data = await res.json();

            if (data.success) {
                setProfileMessage({ text: "Profile updated successfully!", type: "success" });
                setUser(data.user);
            } else {
                setProfileMessage({ text: data.message || "Failed to update profile", type: "error" });
            }
        } catch (err) {
            setProfileMessage({ text: "An error occurred", type: "error" });
        }
    };

    const handleChangePassword = async (e: React.FormEvent) => {
        e.preventDefault();
        setPasswordMessage({ text: "Updating password...", type: "info" });

        try {
            const res = await fetch("/api/auth/password", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ currentPassword, newPassword })
            });
            const data = await res.json();

            if (data.success) {
                setPasswordMessage({ text: "Password changed successfully! You will be logged out.", type: "success" });
                setTimeout(() => {
                    const isIframe = window.self !== window.top;
                    if (isIframe) {
                        window.parent.postMessage({ type: 'AUTH_LOGOUT' }, '*');
                    } else {
                        window.location.href = "/";
                    }
                }, 2000);
            } else {
                setPasswordMessage({ text: data.message || "Failed to change password", type: "error" });
            }
        } catch (err) {
            setPasswordMessage({ text: "An error occurred", type: "error" });
        }
    };

    if (loading) {
        return (
            <div className="flex h-screen items-center justify-center bg-[#111827]">
                <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-indigo-500"></div>
            </div>
        );
    }

    return (
        <div className="min-h-screen flex items-center justify-center bg-[#111827] text-white p-4 font-sans">
            <div className="w-full max-w-md space-y-8">
                <div className="text-center">
                    <div className="mx-auto w-16 h-16 rounded-full bg-gradient-to-tr from-indigo-500 to-purple-500 flex items-center justify-center text-2xl font-black shadow-lg mb-4">
                        {user?.username?.[0]?.toUpperCase() || '?'}
                    </div>
                    <h2 className="text-3xl font-extrabold tracking-tight">Your Profile</h2>
                </div>

                <div className="bg-white/5 backdrop-blur-xl rounded-3xl p-8 border border-white/10 shadow-2xl space-y-8">

                    {/* Profile Update Form */}
                    <form onSubmit={handleUpdateProfile} className="space-y-4">
                        <h3 className="text-lg font-bold text-zinc-300 border-b border-white/10 pb-2">Account Details</h3>

                        {profileMessage.text && (
                            <div className={`p-3 rounded-xl text-sm ${profileMessage.type === 'error' ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20' : profileMessage.type === 'success' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'}`}>
                                {profileMessage.text}
                            </div>
                        )}

                        <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-1">
                                Username
                                {locks.lockUsername && <span className="ml-2 text-amber-400 text-xs font-semibold">Locked</span>}
                            </label>
                            <input
                                type="text"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                disabled={locks.lockUsername}
                                className={`w-full px-4 py-3 bg-black/20 border border-white/10 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all text-white placeholder-zinc-600 outline-none ${locks.lockUsername ? 'opacity-50 cursor-not-allowed' : ''}`}
                                required
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-1">
                                Email Address
                                {locks.lockEmail && <span className="ml-2 text-amber-400 text-xs font-semibold">Locked</span>}
                            </label>
                            <input
                                type="email"
                                value={email}
                                onChange={(e) => setEmail(e.target.value)}
                                disabled={locks.lockEmail}
                                className={`w-full px-4 py-3 bg-black/20 border border-white/10 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all text-white placeholder-zinc-600 outline-none ${locks.lockEmail ? 'opacity-50 cursor-not-allowed' : ''}`}
                                required
                            />
                        </div>
                        {(!locks.lockUsername && !locks.lockEmail) ? (
                            <button
                                type="submit"
                                className="w-full flex justify-center py-3 px-4 border border-transparent rounded-xl shadow-lg text-sm font-bold text-white bg-indigo-600 hover:bg-indigo-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-all"
                            >
                                Update Profile
                            </button>
                        ) : (locks.lockUsername && locks.lockEmail) ? (
                            <p className="text-xs text-amber-400/70 text-center py-2">Profile editing is disabled by your administrator.</p>
                        ) : (
                            <button
                                type="submit"
                                className="w-full flex justify-center py-3 px-4 border border-transparent rounded-xl shadow-lg text-sm font-bold text-white bg-indigo-600 hover:bg-indigo-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-all"
                            >
                                Update Profile
                            </button>
                        )}
                    </form>

                    {/* Password Change Form */}
                    {locks.lockPassword ? (
                        <div className="pt-4 border-t border-white/10">
                            <h3 className="text-lg font-bold text-zinc-300 border-b border-white/10 pb-2">Change Password</h3>
                            <p className="text-sm text-amber-400/70 text-center py-6">Password changes are disabled by your administrator.</p>
                        </div>
                    ) : (
                    <form onSubmit={handleChangePassword} className="space-y-4 pt-4 border-t border-white/10">
                        <h3 className="text-lg font-bold text-zinc-300 border-b border-white/10 pb-2">Change Password</h3>

                        {passwordMessage.text && (
                            <div className={`p-3 rounded-xl text-sm ${passwordMessage.type === 'error' ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20' : passwordMessage.type === 'success' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'}`}>
                                {passwordMessage.text}
                            </div>
                        )}

                        <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-1">Current Password</label>
                            <input
                                type="password"
                                value={currentPassword}
                                onChange={(e) => setCurrentPassword(e.target.value)}
                                className="w-full px-4 py-3 bg-black/20 border border-white/10 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all text-white placeholder-zinc-600 outline-none"
                                required
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-zinc-400 mb-1">New Password</label>
                            <input
                                type="password"
                                value={newPassword}
                                onChange={(e) => setNewPassword(e.target.value)}
                                className="w-full px-4 py-3 bg-black/20 border border-white/10 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all text-white placeholder-zinc-600 outline-none"
                                required
                                minLength={8}
                            />
                        </div>
                        <button
                            type="submit"
                            className="w-full flex justify-center py-3 px-4 border border-transparent rounded-xl shadow-lg text-sm font-bold text-white bg-rose-600 hover:bg-rose-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 transition-all"
                        >
                            Change Password
                        </button>
                    </form>
                    )}

                </div>
            </div>
        </div>
    );
}
