"use client";

import Link from "next/link";
import { useParams, usePathname } from "next/navigation";

export default function ClientAdminLayout({ children }: { children: React.ReactNode }) {
    const params = useParams();
    const clientId = params.clientId as string;
    const pathname = usePathname();

    // User-facing pages (/user/login, /user/register, /user/profile) get NO admin chrome
    const isUserRoute = pathname.includes('/user/');
    if (isUserRoute) {
        return <>{children}</>;
    }

    const navItems = [
        {
            href: `/client/${clientId}`,
            label: "Dashboard",
            exact: true,
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
                </svg>
            ),
        },
        {
            href: `/client/${clientId}/users`,
            label: "Users",
            exact: false,
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                </svg>
            ),
        },
        {
            href: `/client/${clientId}/settings`,
            label: "App Settings",
            exact: false,
            icon: (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
            ),
        },
    ];

    const isActive = (href: string, exact: boolean) =>
        exact ? pathname === href : pathname.startsWith(href);

    return (
        <div className="flex min-h-screen bg-zinc-50 dark:bg-black">
            {/* Sidebar */}
            <aside className="w-64 shrink-0 bg-white dark:bg-zinc-950 border-r border-zinc-200 dark:border-white/10 flex flex-col">
                {/* Logo / App header */}
                <div className="px-6 py-6 border-b border-zinc-200 dark:border-white/10">
                    <p className="text-xs font-bold text-zinc-500 uppercase tracking-widest mb-1">App Admin</p>
                    <p className="text-sm font-mono text-zinc-400 truncate">{clientId}</p>
                </div>

                {/* Navigation */}
                <nav className="flex-1 p-4 space-y-1">
                    {navItems.map((item) => {
                        const active = isActive(item.href, item.exact);
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
                            document.cookie = "client_admin_session=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
                            window.location.href = "/client";
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
