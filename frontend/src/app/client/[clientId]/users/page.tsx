"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";

interface User {
    user_id: string;
    username: string;
    email: string;
    created_at: { seconds: number; nanos: number };
    lock_username: boolean;
    lock_email: boolean;
    lock_password: boolean;
}

type ModalMode = 'create' | 'invite' | 'edit' | null;

const EMPTY_FORM = { username: '', email: '', password: '', lockUsername: false, lockEmail: false, lockPassword: false };

export default function ClientUsersPage() {
    const params = useParams();
    const clientId = params.clientId as string;

    const [users, setUsers] = useState<User[]>([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');

    const [modalMode, setModalMode] = useState<ModalMode>(null);
    const [editingUser, setEditingUser] = useState<User | null>(null);
    const [form, setForm] = useState(EMPTY_FORM);
    const [formError, setFormError] = useState('');
    const [formLoading, setFormLoading] = useState(false);

    // For invite link
    const [inviteLink, setInviteLink] = useState('');
    const [inviteCopied, setInviteCopied] = useState(false);

    const filteredUsers = users.filter(u =>
        u.username.toLowerCase().includes(search.toLowerCase()) ||
        u.email.toLowerCase().includes(search.toLowerCase())
    );

    const fetchUsers = async () => {
        const res = await fetch(`/api/client/${clientId}/users`);
        const data = await res.json();
        if (data.success) setUsers(data.users);
    };

    useEffect(() => {
        if (!clientId) return;
        fetchUsers().finally(() => setLoading(false));
    }, [clientId]);

    const openCreate = () => {
        setForm(EMPTY_FORM);
        setFormError('');
        setInviteLink('');
        setModalMode('create');
    };

    const openInvite = () => {
        setForm(EMPTY_FORM);
        setFormError('');
        setInviteLink('');
        setModalMode('invite');
    };

    const openEdit = (user: User) => {
        setEditingUser(user);
        setForm({
            username: user.username,
            email: user.email,
            password: '',
            lockUsername: user.lock_username || false,
            lockEmail: user.lock_email || false,
            lockPassword: user.lock_password || false,
        });
        setFormError('');
        setModalMode('edit');
    };

    const closeModal = () => {
        setModalMode(null);
        setEditingUser(null);
        setForm(EMPTY_FORM);
        setFormError('');
        setInviteLink('');
    };

    const [inviteLoading, setInviteLoading] = useState(false);

    const generateInviteLink = async () => {
        setInviteLoading(true);
        try {
            const res = await fetch(`/api/client/${clientId}/invite`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email: form.email || '' }),
            });
            const data = await res.json();
            if (data.success && data.inviteToken) {
                const base = `${window.location.origin}/client/${clientId}/user/register`;
                const params = new URLSearchParams({ invite_token: data.inviteToken });
                if (form.email) params.set('email', form.email);
                setInviteLink(`${base}?${params.toString()}`);
            } else {
                setFormError(data.message || 'Failed to generate invite token');
            }
        } catch {
            setFormError('Failed to generate invite token');
        } finally {
            setInviteLoading(false);
        }
    };

    const copyInvite = () => {
        navigator.clipboard.writeText(inviteLink);
        setInviteCopied(true);
        setTimeout(() => setInviteCopied(false), 2000);
    };

    const handleCreate = async (e: React.FormEvent) => {
        e.preventDefault();
        setFormError('');
        setFormLoading(true);
        try {
            const res = await fetch(`/api/client/${clientId}/users`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(form),
            });
            const data = await res.json();
            if (data.success) {
                await fetchUsers();
                closeModal();
            } else {
                setFormError(data.message || "Failed to create user");
            }
        } catch {
            setFormError("An unexpected error occurred");
        } finally {
            setFormLoading(false);
        }
    };

    const handleEdit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!editingUser) return;
        setFormError('');
        setFormLoading(true);
        try {
            const res = await fetch(`/api/client/${clientId}/users/${editingUser.user_id}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    username: form.username,
                    email: form.email,
                    ...(form.password ? { password: form.password } : {}),
                    lockUsername: form.lockUsername,
                    lockEmail: form.lockEmail,
                    lockPassword: form.lockPassword,
                }),
            });
            const data = await res.json();
            if (data.success) {
                await fetchUsers();
                closeModal();
            } else {
                setFormError(data.message || "Failed to update user");
            }
        } catch {
            setFormError("An unexpected error occurred");
        } finally {
            setFormLoading(false);
        }
    };

    const handleDelete = async (userId: string, username: string) => {
        if (!confirm(`Delete user "${username}"? This cannot be undone.`)) return;
        try {
            const res = await fetch(`/api/client/${clientId}/users?userId=${userId}`, { method: 'DELETE' });
            const data = await res.json();
            if (data.success) {
                setUsers(prev => prev.filter(u => u.user_id !== userId));
            } else {
                alert(data.message || "Failed to delete user");
            }
        } catch {
            alert("An error occurred");
        }
    };

    return (
        <div className="p-8 max-w-5xl mx-auto animate-in fade-in slide-in-from-bottom-4 duration-500">
            <header className="mb-8 flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-white">Users</h1>
                    <p className="text-zinc-500 mt-1">Manage all users registered to this application.</p>
                </div>
                <div className="flex gap-3">
                    <button
                        onClick={openInvite}
                        className="flex items-center gap-2 px-5 py-2.5 bg-white dark:bg-white/5 border border-zinc-200 dark:border-white/10 text-zinc-700 dark:text-zinc-300 font-bold rounded-xl shadow-sm hover:border-indigo-400 dark:hover:border-indigo-500 transition-all text-sm"
                    >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                        </svg>
                        Invite Link
                    </button>
                    <button
                        onClick={openCreate}
                        className="flex items-center gap-2 px-5 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white font-bold rounded-xl shadow-sm transition-all text-sm"
                    >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
                        </svg>
                        Create User
                    </button>
                </div>
            </header>

            {/* Search */}
            <div className="mb-6 relative">
                <svg className="absolute left-4 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                <input
                    type="text"
                    placeholder="Search by username or email..."
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    className="w-full pl-10 pr-4 py-3 bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-2xl text-sm text-zinc-900 dark:text-white placeholder-zinc-400 focus:outline-none focus:border-indigo-500 transition-all"
                />
            </div>

            {/* User list */}
            <div className="bg-white dark:bg-white/[0.03] border border-zinc-200 dark:border-white/10 rounded-3xl overflow-hidden shadow-sm">
                <div className="px-6 py-4 border-b border-zinc-100 dark:border-white/5 bg-zinc-50/50 dark:bg-transparent">
                    <h3 className="font-bold text-zinc-900 dark:text-white">{filteredUsers.length} {filteredUsers.length === 1 ? "User" : "Users"}</h3>
                </div>

                {loading ? (
                    <div className="p-16 text-center text-zinc-400">Loading users...</div>
                ) : filteredUsers.length === 0 ? (
                    <div className="p-16 text-center">
                        <svg className="w-12 h-12 mx-auto mb-4 text-zinc-300 dark:text-zinc-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                        </svg>
                        <p className="text-zinc-400 font-medium">{search ? "No users match your search." : "No users yet. Create the first one!"}</p>
                    </div>
                ) : (
                    <div className="divide-y divide-zinc-100 dark:divide-white/5">
                        {filteredUsers.map(u => (
                            <div key={u.user_id} className="flex items-center gap-4 px-6 py-4 hover:bg-zinc-50 dark:hover:bg-white/5 transition-colors group">
                                <div className="w-10 h-10 shrink-0 rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 flex items-center justify-center text-white font-black text-sm shadow-sm">
                                    {u.username[0].toUpperCase()}
                                </div>
                                <div className="flex-1 min-w-0">
                                    <div className="flex items-center gap-1.5">
                                        <p className="font-bold text-zinc-900 dark:text-white text-sm">{u.username}</p>
                                        {(u.lock_username || u.lock_email || u.lock_password) && (
                                            <span title="Has field locks">
                                                <svg className="w-3 h-3 text-amber-500 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                                                </svg>
                                            </span>
                                        )}
                                    </div>
                                    <p className="text-xs text-zinc-400 truncate">{u.email || "No email"}</p>
                                </div>
                                <div className="hidden md:block text-right">
                                    <p className="text-xs font-mono text-zinc-300 dark:text-zinc-600">{u.user_id.slice(0, 8)}…</p>
                                    <p className="text-[10px] text-zinc-400 mt-0.5">
                                        Joined {new Date(u.created_at.seconds * 1000).toLocaleDateString()}
                                    </p>
                                </div>
                                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                    <button
                                        onClick={() => openEdit(u)}
                                        className="p-2 text-zinc-400 hover:text-indigo-500 hover:bg-indigo-50 dark:hover:bg-indigo-500/10 rounded-lg transition-colors"
                                        title="Edit User"
                                    >
                                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                        </svg>
                                    </button>
                                    <button
                                        onClick={() => handleDelete(u.user_id, u.username)}
                                        className="p-2 text-zinc-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 rounded-lg transition-colors"
                                        title="Delete User"
                                    >
                                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                        </svg>
                                    </button>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </div>

            {/* Modal */}
            {modalMode && (
                <div
                    className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4"
                    onClick={e => e.target === e.currentTarget && closeModal()}
                >
                    <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-white/10 rounded-3xl p-8 w-full max-w-md shadow-2xl animate-in zoom-in-95 duration-200">

                        {/* CREATE */}
                        {modalMode === 'create' && (
                            <>
                                <h3 className="text-2xl font-black text-zinc-900 dark:text-white mb-6">Create New User</h3>
                                {formError && <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-xl text-red-500 text-sm font-medium">{formError}</div>}
                                <form onSubmit={handleCreate} className="space-y-4">
                                    <Field label="Username" required type="text" placeholder="e.g. john_doe" value={form.username} onChange={v => setForm({ ...form, username: v })} />
                                    <Field label="Email" type="email" placeholder="john@example.com" value={form.email} onChange={v => setForm({ ...form, email: v })} />
                                    <Field label="Password" required type="password" placeholder="Minimum 8 characters" value={form.password} onChange={v => setForm({ ...form, password: v })} minLength={8} />
                                    <ModalActions onCancel={closeModal} submitLabel={formLoading ? "Creating…" : "Create User"} loading={formLoading} />
                                </form>
                            </>
                        )}

                        {/* INVITE */}
                        {modalMode === 'invite' && (
                            <>
                                <h3 className="text-2xl font-black text-zinc-900 dark:text-white mb-2">Invite User</h3>
                                <p className="text-sm text-zinc-500 mb-6">Generate a registration link to share. The user sets their own password.</p>
                                <div className="space-y-4">
                                    <Field label="Pre-fill Email (optional)" type="email" placeholder="john@example.com" value={form.email} onChange={v => setForm({ ...form, email: v })} />
                                    <button
                                        type="button"
                                        onClick={generateInviteLink}
                                        disabled={inviteLoading}
                                        className="w-full py-3 rounded-xl font-bold text-sm bg-indigo-600 text-white hover:bg-indigo-500 transition-colors disabled:opacity-60"
                                    >
                                        {inviteLoading ? 'Generating...' : 'Generate Link'}
                                    </button>

                                    {inviteLink && (
                                        <div className="mt-2 p-4 bg-zinc-50 dark:bg-black border border-zinc-200 dark:border-white/10 rounded-2xl">
                                            <p className="text-xs font-bold text-zinc-500 uppercase tracking-wider mb-2">Invite Link</p>
                                            <p className="text-xs font-mono text-zinc-700 dark:text-zinc-300 break-all mb-3">{inviteLink}</p>
                                            <button
                                                onClick={copyInvite}
                                                className={`w-full py-2 rounded-xl text-sm font-bold transition-colors ${inviteCopied ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-zinc-900 dark:bg-white text-white dark:text-black hover:opacity-90'}`}
                                            >
                                                {inviteCopied ? "✓ Copied!" : "Copy Link"}
                                            </button>
                                        </div>
                                    )}

                                    <button
                                        type="button"
                                        onClick={closeModal}
                                        className="w-full py-3 rounded-xl font-bold text-sm bg-zinc-100 dark:bg-white/5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-white/10 transition-colors"
                                    >
                                        Done
                                    </button>
                                </div>
                            </>
                        )}

                        {/* EDIT */}
                        {modalMode === 'edit' && editingUser && (
                            <>
                                <h3 className="text-2xl font-black text-zinc-900 dark:text-white mb-1">Edit User</h3>
                                <p className="text-sm text-zinc-500 mb-6">Editing <span className="font-bold text-zinc-700 dark:text-zinc-300">{editingUser.username}</span></p>
                                {formError && <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-xl text-red-500 text-sm font-medium">{formError}</div>}
                                <form onSubmit={handleEdit} className="space-y-4">
                                    <Field label="Username" required type="text" value={form.username} onChange={v => setForm({ ...form, username: v })} />
                                    <Field label="Email" type="email" placeholder="Leave blank to keep current" value={form.email} onChange={v => setForm({ ...form, email: v })} />
                                    <Field label="New Password" type="password" placeholder="Leave blank to keep current" value={form.password} onChange={v => setForm({ ...form, password: v })} minLength={8} />

                                    {/* Field Locks */}
                                    <div className="pt-2 border-t border-zinc-200 dark:border-white/10">
                                        <p className="text-xs font-bold text-zinc-500 uppercase tracking-wider mb-3">User Self-Edit Restrictions</p>
                                        <div className="space-y-2">
                                            <LockToggle label="Lock Username" checked={form.lockUsername} onChange={v => setForm({ ...form, lockUsername: v })} />
                                            <LockToggle label="Lock Email" checked={form.lockEmail} onChange={v => setForm({ ...form, lockEmail: v })} />
                                            <LockToggle label="Lock Password" checked={form.lockPassword} onChange={v => setForm({ ...form, lockPassword: v })} />
                                        </div>
                                        <p className="text-[11px] text-zinc-400 mt-2">Locked fields cannot be changed by the user on their profile page.</p>
                                    </div>

                                    <ModalActions onCancel={closeModal} submitLabel={formLoading ? "Saving…" : "Save Changes"} loading={formLoading} />
                                </form>
                            </>
                        )}

                    </div>
                </div>
            )}
        </div>
    );
}

function Field({ label, required, type, placeholder, value, onChange, minLength }: {
    label: string; required?: boolean; type: string; placeholder?: string;
    value: string; onChange: (v: string) => void; minLength?: number;
}) {
    return (
        <div>
            <label className="block text-sm font-bold text-zinc-700 dark:text-zinc-300 mb-1">
                {label} {required && <span className="text-red-400">*</span>}
            </label>
            <input
                type={type} required={required} placeholder={placeholder} value={value} minLength={minLength}
                onChange={e => onChange(e.target.value)}
                className="w-full bg-zinc-50 dark:bg-black border border-zinc-200 dark:border-white/10 rounded-xl px-4 py-3 text-zinc-900 dark:text-white focus:outline-none focus:border-indigo-500 transition-colors text-sm"
            />
        </div>
    );
}

function ModalActions({ onCancel, submitLabel, loading }: { onCancel: () => void; submitLabel: string; loading: boolean }) {
    return (
        <div className="flex gap-3 pt-2">
            <button type="button" onClick={onCancel} className="flex-1 px-4 py-3 rounded-xl font-bold text-sm bg-zinc-100 dark:bg-white/5 text-zinc-900 dark:text-white hover:bg-zinc-200 dark:hover:bg-white/10 transition-colors">Cancel</button>
            <button type="submit" disabled={loading} className="flex-1 px-4 py-3 rounded-xl font-bold text-sm bg-indigo-600 text-white hover:bg-indigo-500 transition-colors disabled:opacity-60">{submitLabel}</button>
        </div>
    );
}

function LockToggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
    return (
        <label className="flex items-center justify-between py-1.5 px-3 rounded-lg hover:bg-zinc-50 dark:hover:bg-white/5 cursor-pointer transition-colors">
            <span className="text-sm font-medium text-zinc-700 dark:text-zinc-300 flex items-center gap-2">
                <svg className={`w-3.5 h-3.5 ${checked ? 'text-amber-500' : 'text-zinc-300 dark:text-zinc-600'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={checked ? "M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" : "M8 11V7a4 4 0 118 0m-4 8v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2z"} />
                </svg>
                {label}
            </span>
            <div className={`relative w-9 h-5 rounded-full transition-colors ${checked ? 'bg-amber-500' : 'bg-zinc-200 dark:bg-zinc-700'}`}>
                <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${checked ? 'translate-x-4' : 'translate-x-0.5'}`} />
                <input type="checkbox" className="sr-only" checked={checked} onChange={e => onChange(e.target.checked)} />
            </div>
        </label>
    );
}
