"use client";

import { use, Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import LoginForm from "@/components/LoginForm";

interface PageProps {
    params: Promise<{ clientId: string }>;
}

function UserLogin({ clientId }: { clientId: string }) {
    const searchParams = useSearchParams();
    const [config, setConfig] = useState<any>(null);

    useEffect(() => {
        fetch(`/api/client/${clientId}/config`)
            .then(res => res.json())
            .then(data => {
                if (data.success) {
                    setConfig(data.config);
                } else {
                    setConfig({ demo_mode: false, invite_only: false, login_type: 'both' });
                }
            })
            .catch(() => setConfig({ demo_mode: false, invite_only: false, login_type: 'both' }));
    }, [clientId]);

    const redirectUrl = searchParams.get("redirect") || "/dashboard";
    const hideForgot = searchParams.get("hide_forgot") === "true";

    // Config overrides
    const showDemo = config?.demo_mode || false;
    const loginType = config?.login_type || "both";
    const showSignUp = !(config?.invite_only || false);

    const signUpUrl = `/client/${clientId}/user/register?redirect=${encodeURIComponent(redirectUrl)}`;

    if (!config) {
        return <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading Configuration...</div>;
    }

    return (
        <LoginForm
            title="Sign in to continue"
            description="Enter your credentials to access the application"
            clientId={clientId}
            redirectUrl={redirectUrl}
            showSignUp={showSignUp}
            signUpUrl={signUpUrl}
            hideForgot={hideForgot}
            showDemo={showDemo}
            loginType={loginType}
        />
    );
}

export default function LoginPage(props: PageProps) {
    const params = use(props.params);

    return (
        <Suspense fallback={<div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading...</div>}>
            <UserLogin clientId={params.clientId} />
        </Suspense>
    );
}
