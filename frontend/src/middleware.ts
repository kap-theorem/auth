import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export function middleware(request: NextRequest) {
    const { pathname } = request.nextUrl;

    // ───────────────────────────────────────────────
    // REGULAR USER ROUTES — always public, never guard
    // /client/[id]/user/login, /user/register, /user/profile
    // ───────────────────────────────────────────────
    if (/^\/client\/[^/]+\/user(\/|$)/.test(pathname)) {
        return NextResponse.next();
    }

    // ───────────────────────────────────────────────
    // APP ADMIN ROUTES — guard dashboard, /users, /settings
    // /client/[id], /client/[id]/users, /client/[id]/settings
    // ───────────────────────────────────────────────
    const clientAdminRegex = /^\/client\/([^/]+)(\/users|\/settings|\/security)?\/?$/;
    const clientMatch = pathname.match(clientAdminRegex);

    if (clientMatch) {
        const urlClientId = clientMatch[1];

        // Super admins with a valid admin_access_token can bypass
        const superAdminCookie = request.cookies.get('admin_access_token');
        if (superAdminCookie) {
            return NextResponse.next();
        }

        const adminCookie = request.cookies.get('client_admin_session');
        if (!adminCookie || adminCookie.value !== urlClientId) {
            console.log(`[Middleware] Blocked unauthorized access to App Admin: ${urlClientId}`);
            return NextResponse.redirect(new URL('/client', request.url));
        }
        return NextResponse.next();
    }

    // ───────────────────────────────────────────────
    // SUPER ADMIN ROUTES
    // ───────────────────────────────────────────────
    if (pathname.startsWith('/admin/dashboard')) {
        const superAdminCookie = request.cookies.get('admin_access_token');
        if (!superAdminCookie) {
            console.log(`[Middleware] Blocked unauthorized access to Super Admin Dashboard`);
            return NextResponse.redirect(new URL('/admin', request.url));
        }
        return NextResponse.next();
    }

    return NextResponse.next();
}

export const config = {
    matcher: ['/client/:path*', '/admin/:path*'],
};
