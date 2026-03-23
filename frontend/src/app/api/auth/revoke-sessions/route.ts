import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    const accessToken = req.cookies.get('access_token')?.value;

    if (!accessToken) {
        return NextResponse.json({ success: false, message: "Unauthorized" }, { status: 401 });
    }

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.RevokeAllSessions(
            { access_token: accessToken },
            (error: any, response: any) => {
                if (error) {
                    console.error("gRPC RevokeAllSessions Error:", error);
                    resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                    return;
                }
                resolve(NextResponse.json({
                    success: response.success,
                    message: response.message,
                    revokedCount: response.revoked_count,
                }));
            }
        );
    });
}
