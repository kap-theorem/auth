"use client";

import { Suspense } from "react";
import LoginForm from "@/components/LoginForm";

function AdminLogin() {
    const AUTH_ADMIN_CLIENT_ID = process.env.NEXT_PUBLIC_AUTH_ADMIN_CLIENT_ID || "218a88fe-a3f3-4718-8f96-f55472c40139";

    return (
        <LoginForm
            title="Kaplabs Central Administration"
            description="Super Administrator Login"
            clientId={AUTH_ADMIN_CLIENT_ID}
            redirectUrl="/admin/dashboard"
            showSignUp={false} // Super admins can't sign up, they must be provisioned
        />
    );
}

export default function AdminLoginPage() {
    return (
        <Suspense fallback={<div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading...</div>}>
            <AdminLogin />
        </Suspense>
    );
}
