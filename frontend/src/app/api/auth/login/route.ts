import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

const AUTH_ADMIN_CLIENT_ID = process.env.AUTH_ADMIN_CLIENT_ID || "218a88fe-a3f3-4718-8f96-f55472c40139";

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    try {
        const { loginIdentifier, password, clientId } = await req.json();

        if (!loginIdentifier || !password || !clientId) {
            return NextResponse.json({ success: false, message: "Missing required fields" }, { status: 400 });
        }

        const isSuperAdmin = clientId === AUTH_ADMIN_CLIENT_ID;

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.GetToken(
                { login_identifier: loginIdentifier, password, client_id: clientId, user_agent: req.headers.get('user-agent') || '' },
                (error: any, response: any) => {
                    if (error) {
                        console.error("gRPC Login Error:", error);
                        resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                        return;
                    }

                    if (response.success) {
                        const res = NextResponse.json(response);
                        const isProduction = process.env.NODE_ENV === 'production';
                        const cookieDomain = isProduction ? 'auth.kaplabs.dev' : undefined;

                        if (isSuperAdmin) {
                            // Super Admin gets a separate cookie with 1-week TTL
                            res.cookies.set('admin_access_token', response.access_token, {
                                httpOnly: true,
                                secure: isProduction,
                                sameSite: isProduction ? 'none' : 'lax',
                                path: '/',
                                domain: cookieDomain,
                                maxAge: 7 * 24 * 60 * 60, // 1 week
                            });
                        } else {
                            // Regular user gets access_token + refresh_token
                            res.cookies.set('access_token', response.access_token, {
                                httpOnly: true,
                                secure: isProduction,
                                sameSite: isProduction ? 'none' : 'lax',
                                path: '/',
                                domain: cookieDomain,
                                maxAge: 30 * 60, // 30 minutes
                            });

                            res.cookies.set('refresh_token', response.refresh_token, {
                                httpOnly: true,
                                secure: isProduction,
                                sameSite: isProduction ? 'none' : 'lax',
                                path: '/api/auth/refresh',
                                domain: cookieDomain,
                                maxAge: 7 * 24 * 60 * 60, // 7 days
                            });
                        }

                        resolve(res);
                    } else {
                        resolve(NextResponse.json({ success: false, message: response.message }, { status: 401 }));
                    }
                }
            );
        });
    } catch (error) {
        return NextResponse.json({ success: false, message: "Invalid request payload" }, { status: 400 });
    }
}
