"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export default function SuperAdminLayout({ children }: { children: React.ReactNode }) {
    const pathname = usePathname();

    const navItems = [
        {
            href: "/admin/dashboard",
            label: "Dashboard",
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
                </svg>
            ),
        },
    ];

    const isActive = (href: string) => pathname.startsWith(href);

    return (
        <div className="flex min-h-screen bg-zinc-50 dark:bg-black">
            {/* Sidebar */}
            <aside className="w-64 shrink-0 bg-white dark:bg-zinc-950 border-r border-zinc-200 dark:border-white/10 flex flex-col">
                {/* Header */}
                <div className="px-6 py-6 border-b border-zinc-200 dark:border-white/10">
                    <p className="text-xs font-bold text-zinc-500 uppercase tracking-widest mb-1">Kaplabs Auth</p>
                    <p className="text-sm font-bold text-zinc-900 dark:text-white">Super Admin</p>
                    <span className="inline-flex items-center gap-1.5 mt-2 text-[10px] font-bold px-2 py-0.5 bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 rounded-full uppercase tracking-wider">
                        <span className="w-1.5 h-1.5 rounded-full bg-indigo-500 animate-pulse" />
                        System Access
                    </span>
                </div>

                {/* Nav */}
                <nav className="flex-1 p-4 space-y-1">
                    {navItems.map((item) => {
                        const active = isActive(item.href);
                        return (
                            <Link
                                key={item.href}
                                href={item.href}
                                className={`flex items-center gap-3 px-4 py-2.5 rounded-xl font-semibold text-sm transition-all ${active
                                        ? "bg-indigo-500 text-white shadow-sm"
                                        : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-white/5 hover:text-zinc-900 dark:hover:text-white"
                                    }`}
                            >
                                {item.icon}
                                {item.label}
                            </Link>
                        );
                    })}
                </nav>

                {/* Sign out */}
                <div className="p-4 border-t border-zinc-200 dark:border-white/10">
                    <button
                        onClick={() => {
                            document.cookie = "admin_access_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
                            window.location.href = "/admin";
                        }}
                        className="w-full flex items-center gap-3 px-4 py-2.5 rounded-xl font-semibold text-sm text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-500/10 transition-all"
                    >
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                        </svg>
                        Sign Out
                    </button>
                </div>
            </aside>

            {/* Main content */}
            <main className="flex-1 overflow-auto">
                {children}
            </main>
        </div>
    );
}
