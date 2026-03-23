import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    const refreshToken = req.cookies.get('refresh_token')?.value;

    if (!refreshToken) {
        return NextResponse.json({ success: false, message: "No refresh token" }, { status: 401 });
    }

    let clientId: string;
    try {
        const body = await req.json();
        clientId = body.clientId;
    } catch {
        return NextResponse.json({ success: false, message: "clientId is required" }, { status: 400 });
    }

    if (!clientId) {
        return NextResponse.json({ success: false, message: "clientId is required" }, { status: 400 });
    }

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.RefreshToken(
            { refresh_token: refreshToken, client_id: clientId },
            (error: any, response: any) => {
                if (error) {
                    console.error("gRPC RefreshToken Error:", error);
                    resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                    return;
                }

                if (response.success) {
                    const res = NextResponse.json({ success: true });
                    const isProduction = process.env.NODE_ENV === 'production';
                    const cookieDomain = isProduction ? 'auth.kaplabs.dev' : undefined;

                    res.cookies.set('access_token', response.access_token, {
                        httpOnly: true,
                        secure: isProduction,
                        sameSite: isProduction ? 'none' : 'lax',
                        path: '/',
                        domain: cookieDomain,
                        maxAge: 30 * 60,
                    });

                    res.cookies.set('refresh_token', response.refresh_token, {
                        httpOnly: true,
                        secure: isProduction,
                        sameSite: isProduction ? 'none' : 'lax',
                        path: '/api/auth/refresh',
                        domain: cookieDomain,
                        maxAge: 7 * 24 * 60 * 60,
                    });

                    resolve(res);
                } else {
                    resolve(NextResponse.json({ success: false, message: response.message }, { status: 401 }));
                }
            }
        );
    });
}
