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
    const [clientName, setClientName] = useState<string>("");

    useEffect(() => {
        fetch(`/api/client/${clientId}/config`)
            .then(res => res.json())
            .then(data => {
                if (data.success) {
                    setConfig(data.config);
                    setClientName(data.client_name || "");
                } else {
                    setConfig({ demo_mode: false, invite_only: false, login_type: 'both' });
                }
            })
            .catch(() => setConfig({ demo_mode: false, invite_only: false, login_type: 'both' }));
    }, [clientId]);

    const redirectUrl = searchParams.get("redirect") || "/dashboard";
    // Config overrides
    const showDemo = config?.demo_mode || false;
    const loginType = config?.login_type || "both";
    const showSignUp = !(config?.invite_only || false);

    const signUpUrl = `/client/${clientId}/user/register?redirect=${encodeURIComponent(redirectUrl)}`;

    if (!config) {
        return <div className="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-black text-zinc-500">Loading Configuration...</div>;
    }

    const title = clientName
        ? `Sign in to ${clientName}`
        : "Sign in to continue";

    return (
        <LoginForm
            title={title}
            description="Authentication powered by auth.kaplabs.dev"
            clientId={clientId}
            redirectUrl={redirectUrl}
            showSignUp={showSignUp}
            signUpUrl={signUpUrl}
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
