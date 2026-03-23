import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    try {
        const { email, clientId } = await req.json();

        if (!email || !clientId) {
            return NextResponse.json({ success: false, message: "Email and client ID are required" }, { status: 400 });
        }

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.RequestPasswordReset(
                { email, client_id: clientId },
                (error: any, response: any) => {
                    if (error) {
                        console.error("gRPC RequestPasswordReset Error:", error);
                        resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                        return;
                    }
                    resolve(NextResponse.json({
                        success: response.success,
                        message: response.message,
                        resetToken: response.reset_token || undefined,
                    }));
                }
            );
        });
    } catch {
        return NextResponse.json({ success: false, message: "Invalid request payload" }, { status: 400 });
    }
}
