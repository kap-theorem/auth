import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function GET(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    const accessToken = req.cookies.get('access_token')?.value;

    if (!accessToken) {
        return NextResponse.json({ authenticated: false });
    }

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.ValidateToken({ access_token: accessToken }, (err: any, response: any) => {
            if (err || !response?.valid) {
                resolve(NextResponse.json({ authenticated: false }));
                return;
            }

            resolve(NextResponse.json({
                authenticated: true,
                user: {
                    userId: response.user?.user_id || "",
                    username: response.user?.username || "",
                    email: response.user?.email || "",
                    clientId: response.user?.client_id || "",
                    lockUsername: response.user?.lock_username || false,
                    lockEmail: response.user?.lock_email || false,
                    lockPassword: response.user?.lock_password || false,
                }
            }));
        });
    });
}
